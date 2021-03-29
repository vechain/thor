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
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type bsWithBeta struct {
	sig  block.ComplexSignature
	beta []byte
}

// Flow the flow of packing a new block.
type Flow struct {
	packer            *Packer
	parentHeader      *block.Header
	runtime           *runtime.Runtime
	processedTxs      map[thor.Bytes32]bool // txID -> reverted
	gasUsed           uint64
	txs               tx.Transactions
	receipts          tx.Receipts
	features          tx.Features
	proposers         []poa.Proposer // all proposers in power
	maxBlockProposers uint64
	alpha             []byte
	knownBackers      map[thor.Address]bool
	bss               []bsWithBeta
}

func newFlow(
	packer *Packer,
	parentHeader *block.Header,
	runtime *runtime.Runtime,
	features tx.Features,
	proposers []poa.Proposer,
	maxBlockProposers uint64,
	seed []byte,
) *Flow {
	alpha := append([]byte(nil), seed...)
	alpha = append(alpha, parentHeader.ID().Bytes()[:4]...)

	return &Flow{
		packer:            packer,
		parentHeader:      parentHeader,
		runtime:           runtime,
		processedTxs:      make(map[thor.Bytes32]bool),
		features:          features,
		proposers:         proposers,
		maxBlockProposers: maxBlockProposers,
		alpha:             alpha,
		knownBackers:      make(map[thor.Address]bool),
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

// Number returns the block number.
func (f *Flow) Number() uint32 {
	return f.runtime.Context().Number
}

// TotalScore returns total score of new block.
func (f *Flow) TotalScore() uint64 {
	return f.runtime.Context().TotalScore
}

// GetAuthority returns authority corresponding to the given address.
func (f *Flow) GetAuthority(addr thor.Address) *poa.Proposer {
	for _, p := range f.proposers {
		if p.Address == addr {
			return &poa.Proposer{
				Address: p.Address,
				Active:  p.Active,
			}
		}
	}
	return nil
}

// Alpha returns the alpha of this round.
func (f *Flow) Alpha() []byte {
	return f.alpha
}

// IsBackerKnown returns true is backer's signature is already added.
func (f *Flow) IsBackerKnown(backer thor.Address) bool {
	return f.knownBackers[backer]
}

// MaxBlockProposers returns the max block proposers count.
func (f *Flow) MaxBlockProposers() uint64 {
	return f.maxBlockProposers
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

// Draft an block proposal and sign it for p2p propagation.
func (f *Flow) Draft(privateKey *ecdsa.PrivateKey) (*proto.Draft, error) {
	if f.packer.nodeMaster != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
		return nil, errors.New("private key mismatch")
	}

	p := block.NewProposal(f.parentHeader.ID(), f.txs.RootHash(), f.runtime.Context().GasLimit, f.runtime.Context().Time)
	sig, err := crypto.Sign(p.Hash().Bytes(), privateKey)
	if err != nil {
		return nil, err
	}

	draft := proto.Draft{
		Proposal:  p,
		Signature: sig,
	}
	return &draft, nil
}

// AddBackerSignature adds a backer signature.
func (f *Flow) AddBackerSignature(bs block.ComplexSignature, beta []byte, signer thor.Address) bool {
	if signer == f.packer.nodeMaster {
		return false
	}

	cpy := bs
	f.knownBackers[signer] = true
	f.bss = append(f.bss, bsWithBeta{cpy, beta})

	return true
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
		var bss block.ComplexSignatures
		if len(f.bss) > 0 {
			sort.Slice(f.bss, func(i, j int) bool {
				return bytes.Compare(f.bss[i].beta, f.bss[j].beta) < 0
			})
			for _, b := range f.bss {
				bss = append(bss, b.sig)
			}
		}

		builder.Alpha(f.alpha).BackerSignatures(bss, f.parentHeader.TotalQuality())
	}

	for _, tx := range f.txs {
		builder.Transaction(tx)
	}
	newBlock := builder.Build()

	var signature []byte
	sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if f.runtime.Context().Number >= f.packer.forkConfig.VIP193 {
		var proof []byte
		_, proof, err = ecvrf.NewSecp256k1Sha256Tai().Prove(privateKey, f.alpha)
		if err != nil {
			return nil, nil, nil, err
		}
		cs, err := block.NewComplexSignature(proof, sig)
		if err != nil {
			return nil, nil, nil, err
		}
		signature = cs
	} else {
		signature = sig
	}

	return newBlock.WithSignature(signature), stage, f.receipts, nil
}
