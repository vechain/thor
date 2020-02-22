// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

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

	blockSummary *block.Summary
	// txSet        *block.TxSet
	endorsements block.Endorsements

	blockClose chan interface{}
}

// NewFlow ...
func NewFlow(
	packer *Packer,
	parentHeader *block.Header,
	runtime *runtime.Runtime,
	features tx.Features,
) *Flow {
	f := &Flow{
		packer:       packer,
		parentHeader: parentHeader,
		runtime:      runtime,
		processedTxs: make(map[thor.Bytes32]bool),
		features:     features,
		blockClose:   make(chan interface{}, 1),
	}
	// go func() {
	// 	f.blockClose <- struct{}{}
	// }()
	return f
}

// Close waits for blockClose and set packer == nil
func (f *Flow) Close() {
	f.blockClose <- struct{}{}

	f.packer = nil

	return
}

// // SetBlockSummary ...
// func (f *Flow) SetBlockSummary(bs *block.Summary) {
// 	f.blockSummary = bs.Copy()
// }

// GetBlockSummary ...
func (f *Flow) GetBlockSummary() *block.Summary {
	return f.blockSummary.Copy()
}

// HasPackedBlockSummary ...
func (f *Flow) HasPackedBlockSummary() bool {
	return f.blockSummary != nil
}

// IsEmpty ...
func (f *Flow) IsEmpty() bool {
	return f.packer == nil
}

// AddEndoresement stores an endorsement
func (f *Flow) AddEndoresement(ed *block.Endorsement) bool {
	return f.endorsements.AddEndorsement(ed)
}

// NumOfEndorsements returns how many endorsements having been stored
func (f *Flow) NumOfEndorsements() int {
	return f.endorsements.Len()
}

// // Txs ...
// func (f *Flow) Txs() tx.Transactions {
// 	return f.txs
// }

// PackTxSetAndBlockSummary packs the tx set and block summary
func (f *Flow) PackTxSetAndBlockSummary(sk *ecdsa.PrivateKey) (bs *block.Summary, ts *block.TxSet, err error) {
	var sig []byte

	if f.packer.nodeMaster != thor.Address(crypto.PubkeyToAddress(sk.PublicKey)) {
		return nil, nil, errors.New("private key mismatch")
	}

	// pack tx set
	if len(f.txs) > 0 {
		ts = block.NewTxSet(f.txs, f.runtime.Context().Time, f.runtime.Context().TotalScore)
		sig, err = crypto.Sign(ts.SigningHash().Bytes(), sk)
		if err != nil {
			return nil, nil, err
		}
		ts = ts.WithSignature(sig)
	}

	// pack block summary
	best := f.packer.repo.BestBlock()
	parent := best.Header().ID()
	var root thor.Bytes32
	if ts != nil {
		root = ts.TxsRoot()
	} else {
		root = tx.EmptyRoot
	}

	bs = block.NewBlockSummary(parent, root, f.runtime.Context().Time, f.runtime.Context().TotalScore)
	sig, err = crypto.Sign(bs.SigningHash().Bytes(), sk)
	if err != nil {
		return nil, nil, err
	}
	bs = bs.WithSignature(sig)

	f.blockSummary = bs
	// f.txSet = ts

	return
}

// ParentHeader returns parent block header.
func (f *Flow) ParentHeader() *block.Header {
	return f.parentHeader
}

// When the target time to do packing.
func (f *Flow) When() uint64 {
	return f.runtime.Context().Time
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
	// f.Adopt(tx) is called by the go routine that executes func (n *Node) adoptTxs(ctx, flow).
	// It is possible that flow is closed while this function is still on. Therefore, we need to
	// block f.Close() during the execution of f.Adopt(tx)
	f.blockClose <- struct{}{}
	defer func() { <-f.blockClose }()

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
	vip193 := f.packer.forkConfig.VIP193
	if vip193 == 0 {
		vip193 = 1
	}
	isVip193 := f.parentHeader.Number()+1 >= vip193

	if !isVip193 {
		return f.pack(privateKey)
	}

	return f.pack2(privateKey)
}

func (f *Flow) pack(privateKey *ecdsa.PrivateKey) (*block.Block, *state.Stage, tx.Receipts, error) {
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

// vip193
func (f *Flow) pack2(sk *ecdsa.PrivateKey) (*block.Block, *state.Stage, tx.Receipts, error) {
	if f.blockSummary == nil {
		return nil, nil, nil, errors.New("empty block summary")
	}

	if f.packer.nodeMaster != thor.Address(crypto.PubkeyToAddress(sk.PublicKey)) {
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
		TransactionFeatures(f.features).
		VrfProofs(f.endorsements.VrfProofs()).
		SigOnBlockSummary(f.blockSummary.Signature()).
		SigsOnEndorsement(f.endorsements.Signatures())

	for _, tx := range f.txs {
		builder.Transaction(tx)
	}
	newBlock := builder.Build()

	sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), sk)
	if err != nil {
		return nil, nil, nil, err
	}

	return newBlock.WithSignature(sig), stage, f.receipts, nil
}
