// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vrf"
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
	}
}

// ParentHeader returns parent block header.
func (f *Flow) ParentHeader() *block.Header {
	return f.parentHeader
}

// Number returns new block number.
func (f *Flow) Number() uint32 {
	return f.runtime.Context().Number
}

// When the target time to do packing.
func (f *Flow) When() uint64 {
	return f.runtime.Context().Time
}

// TotalScore returns total score of new block.
func (f *Flow) TotalScore() uint64 {
	return f.runtime.Context().TotalScore
}

func (f *Flow) findDep(txID thor.Bytes32) (found bool, reverted bool, err error) {
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

func (f *Flow) hasTx(txid thor.Bytes32, txBlockRef uint32) (bool, error) {
	if _, has := f.processedTxs[txid]; has {
		return true, nil
	}
	return f.runtime.Chain().HasTransaction(txid, txBlockRef)
}

// Adopt try to execute the given transaction.
// If the tx is valid and can be executed on current state (regardless of VM error),
// it will be adopted by the new block.
func (f *Flow) Adopt(t *tx.Transaction) error {
	origin, _ := t.Origin()
	if f.Number() >= f.packer.forkConfig.BLOCKLIST && thor.IsOriginBlocked(origin) {
		return badTxError{"tx origin blocked"}
	}

	delegator, err := t.Delegator()
	if err != nil {
		return badTxError{"delegator cannot be extracted"}
	}
	if f.Number() >= f.packer.forkConfig.BLOCKLIST && delegator != nil && thor.IsOriginBlocked(*delegator) {
		return badTxError{"tx delegator blocked"}
	}

	if err := t.TestFeatures(f.features); err != nil {
		return badTxError{err.Error()}
	}

	switch {
	case t.ChainTag() != f.packer.repo.ChainTag():
		return badTxError{"chain tag mismatch"}
	case f.Number() < t.BlockRef().Number():
		return errTxNotAdoptableNow
	case t.IsExpired(f.Number()):
		return badTxError{"expired"}
	case f.gasUsed+t.Gas() > f.runtime.Context().GasLimit:
		// has enough space to adopt minimum tx
		if f.gasUsed+thor.TxGas+thor.ClauseGas <= f.runtime.Context().GasLimit {
			// try to find a lower gas tx
			return errTxNotAdoptableNow
		}
		return errGasLimitReached
	}

	if f.Number() < f.packer.forkConfig.GALACTICA {
		if t.Type() != tx.TypeLegacy {
			return badTxError{"invalid tx type"}
		}
	} else {
		if f.runtime.Context().BaseFee == nil {
			return fork.ErrBaseFeeNotSet
		}
		baseGasPrice, err := builtin.Params.Native(f.runtime.State()).Get(thor.KeyBaseGasPrice)
		if err != nil {
			return err
		}
		if err := fork.ValidateGalacticaTxFee(t, f.runtime.Context().BaseFee, baseGasPrice); err != nil {
			return errTxNotAdoptableNow
		}
	}

	// check if tx already there
	if found, err := f.hasTx(t.ID(), t.BlockRef().Number()); err != nil {
		return err
	} else if found {
		return errKnownTx
	}

	if dependsOn := t.DependsOn(); dependsOn != nil {
		// check if deps exists
		found, reverted, err := f.findDep(*dependsOn)
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
	receipt, err := f.runtime.ExecuteTransaction(t)
	if err != nil {
		// skip and revert state
		f.runtime.State().RevertTo(checkpoint)
		return badTxError{err.Error()}
	}
	f.processedTxs[t.ID()] = receipt.Reverted
	f.gasUsed += receipt.GasUsed
	f.receipts = append(f.receipts, receipt)
	f.txs = append(f.txs, t)
	return nil
}

// Pack build and sign the new block.
func (f *Flow) Pack(privateKey *ecdsa.PrivateKey, newBlockConflicts uint32, shouldVote bool) (*block.Block, *state.Stage, tx.Receipts, error) {
	if f.packer.nodeMaster != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
		return nil, nil, nil, errors.New("private key mismatch")
	}

	stage, err := f.runtime.State().Stage(trie.Version{Major: f.Number(), Minor: newBlockConflicts})
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

	if f.Number() >= f.packer.forkConfig.FINALITY && shouldVote {
		builder.COM()
	}

	if f.Number() >= f.packer.forkConfig.GALACTICA {
		builder.BaseFee(fork.CalcBaseFee(f.packer.forkConfig, f.parentHeader))
	}

	if f.Number() < f.packer.forkConfig.VIP214 {
		newBlock := builder.Build()

		sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
		if err != nil {
			return nil, nil, nil, err
		}
		return newBlock.WithSignature(sig), stage, f.receipts, nil
	} else {
		parentBeta, err := f.parentHeader.Beta()
		if err != nil {
			return nil, nil, nil, err
		}

		var alpha []byte
		// initial value of chained VRF
		if len(parentBeta) == 0 {
			alpha = f.parentHeader.StateRoot().Bytes()
		} else {
			alpha = parentBeta
		}

		newBlock := builder.Alpha(alpha).Build()
		ec, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
		if err != nil {
			return nil, nil, nil, err
		}

		_, proof, err := vrf.Prove(privateKey, alpha)
		if err != nil {
			return nil, nil, nil, err
		}
		sig, err := block.NewComplexSignature(ec, proof)
		if err != nil {
			return nil, nil, nil, err
		}

		return newBlock.WithSignature(sig), stage, f.receipts, nil
	}
}
