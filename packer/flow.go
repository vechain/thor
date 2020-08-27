// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"bytes"
	"crypto/ecdsa"
	"sort"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type bsWithBeta struct {
	bs   *block.VRFSignature
	beta []byte
}

// Flow the flow of packing a new block.
type Flow struct {
	packer       *Packer
	parentHeader *block.Header
	runtime      *runtime.Runtime
	processedTxs map[thor.Bytes32]bool // txID -> reverted
	gasUsed      uint64
	txs          tx.Transactions
	receipts     tx.Receipts
	features     tx.Features
	knownBackers []thor.Address
	bss          []bsWithBeta
}

func newFlow(
	packer *Packer,
	parentHeader *block.Header,
	runtime *runtime.Runtime,
	features tx.Features,
) *Flow {
	return &Flow{
		packer:       packer,
		parentHeader: parentHeader,
		runtime:      runtime,
		processedTxs: make(map[thor.Bytes32]bool),
		features:     features,
		knownBackers: []thor.Address{},
		bss:          []bsWithBeta{},
	}
}

// ParentHeader returns parent block header.
func (f *Flow) ParentHeader() *block.Header {
	return f.parentHeader
}

// When the target time to do packing.
func (f *Flow) When() uint64 {
	return f.runtime.Context().Time
}

// Number returns the block number to pack.
func (f *Flow) Number() uint32 {
	return f.runtime.Context().Number
}

// TotalScore returns total score of new block.
func (f *Flow) TotalScore() uint64 {
	return f.runtime.Context().TotalScore
}

func (f *Flow) findTx(txID thor.Bytes32) (found bool, reverted bool, err error) {
	if reverted, ok := f.processedTxs[txID]; ok {
		return true, reverted, nil
	}
	txMeta, err := f.runtime.Chain().GetTransactionMeta(txID)
	if err != nil {
		if f.packer.repo.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}
	return true, txMeta.Reverted, nil
}

// Adopt try to execute the given transaction.
// If the tx is valid and can be executed on current state (regardless of VM error),
// it will be adopted by the new block.
func (f *Flow) Adopt(tx *tx.Transaction) error {
	origin, _ := tx.Origin()
	if f.runtime.Context().Number >= f.packer.forkConfig.BLOCKLIST && thor.IsOriginBlocked(origin) {
		return badTxError{"tx origin blocked"}
	}

	if err := tx.TestFeatures(f.features); err != nil {
		return badTxError{err.Error()}
	}

	switch {
	case tx.ChainTag() != f.packer.repo.ChainTag():
		return badTxError{"chain tag mismatch"}
	case f.runtime.Context().Number < tx.BlockRef().Number():
		return errTxNotAdoptableNow
	case tx.IsExpired(f.runtime.Context().Number):
		return badTxError{"expired"}
	case f.gasUsed+tx.Gas() > f.runtime.Context().GasLimit:
		// has enough space to adopt minimum tx
		if f.gasUsed+thor.TxGas+thor.ClauseGas <= f.runtime.Context().GasLimit {
			// try to find a lower gas tx
			return errTxNotAdoptableNow
		}
		return errGasLimitReached
	}

	// check if tx already there
	if found, _, err := f.findTx(tx.ID()); err != nil {
		return err
	} else if found {
		return errKnownTx
	}

	if dependsOn := tx.DependsOn(); dependsOn != nil {
		// check if deps exists
		found, reverted, err := f.findTx(*dependsOn)
		if err != nil {
			return err
		}
		if !found {
			return errTxNotAdoptableNow
		}
		if reverted {
			return errTxNotAdoptableForever
		}
	}

	checkpoint := f.runtime.State().NewCheckpoint()
	receipt, err := f.runtime.ExecuteTransaction(tx)
	if err != nil {
		// skip and revert state
		f.runtime.State().RevertTo(checkpoint)
		return badTxError{err.Error()}
	}
	f.processedTxs[tx.ID()] = receipt.Reverted
	f.gasUsed += receipt.GasUsed
	f.receipts = append(f.receipts, receipt)
	f.txs = append(f.txs, tx)
	return nil
}

// Declare a block declaration with block meta and txs root hash.
func (f *Flow) Declare(privateKey *ecdsa.PrivateKey) (*block.Declaration, error) {
	if f.packer.nodeMaster != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
		return nil, errors.New("private key mismatch")
	}

	dec := block.NewDeclaration(f.parentHeader.ID(), f.txs.RootHash(), f.runtime.Context().GasLimit, f.runtime.Context().Time)
	sig, err := crypto.Sign(dec.SigningHash().Bytes(), privateKey)
	if err != nil {
		return nil, err
	}

	return dec.WithSignature(sig), nil
}

// IsBackerKnown returns true is backer's signature is already added.
func (f *Flow) IsBackerKnown(addr thor.Address) bool {
	for _, backer := range f.knownBackers {
		if addr == backer {
			return true
		}
	}
	return false
}

// AddBackerSignature adds a signature from backer.
func (f *Flow) AddBackerSignature(bs *block.VRFSignature, beta []byte) bool {
	signer, _ := bs.Signer()
	if f.IsBackerKnown(signer) == true {
		return false
	}

	if signer == f.packer.nodeMaster {
		return false
	}

	cpy := *bs
	f.knownBackers = append(f.knownBackers, signer)
	f.bss = append(f.bss, bsWithBeta{&cpy, beta})
	return true
}

// Pack build and sign the new block.
func (f *Flow) Pack(privateKey *ecdsa.PrivateKey) (*block.Block, *state.Stage, tx.Receipts, error) {
	if f.packer.nodeMaster != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
		return nil, nil, nil, errors.New("private key mismatch")
	}

	stage, err := f.runtime.State().Stage()
	if err != nil {
		return nil, nil, nil, err
	}
	stateRoot := stage.Hash()

	builder := new(block.Builder).
		Beneficiary(f.runtime.Context().Beneficiary).
		GasLimit(f.runtime.Context().GasLimit).
		ParentID(f.parentHeader.ID()).
		Timestamp(f.runtime.Context().Time).
		TotalScore(f.runtime.Context().TotalScore).
		GasUsed(f.gasUsed).
		ReceiptsRoot(f.receipts.RootHash()).
		StateRoot(stateRoot).
		TransactionFeatures(f.features)

	if f.runtime.Context().Number >= f.packer.forkConfig.VIP193 {
		var bss block.VRFSignatures
		if len(f.bss) > 0 {
			sort.Slice(f.bss, func(i, j int) bool {
				return bytes.Compare(f.bss[i].beta, f.bss[j].beta) < 0
			})
			for _, b := range f.bss {
				bss = append(bss, b.bs)
			}
		}

		builder.BackerSignatures(bss, f.parentHeader.TotalBackersCount())
	}

	for _, tx := range f.txs {
		builder.Transaction(tx)
	}
	newBlock := builder.Build()

	sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
	if err != nil {
		return nil, nil, nil, err
	}
	return newBlock.WithSignature(sig), stage, f.receipts, nil
}
