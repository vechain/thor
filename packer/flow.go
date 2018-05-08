package packer

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
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
	traverser    *chain.Traverser
	processedTxs map[thor.Bytes32]bool // txID -> reverted
	totalScore   uint64
	gasUsed      uint64
	txs          tx.Transactions
	receipts     tx.Receipts
}

func newFlow(
	packer *Packer,
	parentHeader *block.Header,
	runtime *runtime.Runtime,
	totalScore uint64,
	traverser *chain.Traverser,
) *Flow {
	return &Flow{
		packer:       packer,
		parentHeader: parentHeader,
		runtime:      runtime,
		traverser:    traverser,
		processedTxs: make(map[thor.Bytes32]bool),
		totalScore:   totalScore,
	}
}

// ParentHeader returns parent block header.
func (f *Flow) ParentHeader() *block.Header {
	return f.parentHeader
}

// When the target time to do packing.
func (f *Flow) When() uint64 {
	return f.runtime.BlockTime()
}

// Adopt try to execute the given transaction.
// If the tx is valid and can be executed on current state (regardless of VM error),
// it will be adopted by the new block.
func (f *Flow) Adopt(tx *tx.Transaction) error {
	switch {
	case tx.ChainTag() != f.packer.chain.Tag():
		return badTxError{"chain tag mismatch"}
	case tx.HasReservedFields():
		return badTxError{"reserved fields not empty"}
	case f.runtime.BlockNumber() < tx.BlockRef().Number():
		return errTxNotAdoptableNow
	case tx.IsExpired(f.runtime.BlockNumber()):
		return badTxError{"expired"}
	case f.gasUsed+tx.Gas() > f.runtime.BlockGasLimit():
		// gasUsed < 90% gas limit
		if float64(f.gasUsed)/float64(f.runtime.BlockGasLimit()) < 0.9 {
			// try to find a lower gas tx
			return errTxNotAdoptableNow
		}
		return errGasLimitReached
	}
	// check if tx already there
	if found, _, err := consensus.FindTransaction(f.packer.chain, f.parentHeader.ID(), f.processedTxs, tx.ID()); err != nil {
		return err
	} else if found {
		return errKnownTx
	}

	if dependsOn := tx.DependsOn(); dependsOn != nil {
		// check if deps exists
		found, isReverted, err := consensus.FindTransaction(f.packer.chain, f.parentHeader.ID(), f.processedTxs, *dependsOn)
		if err != nil {
			return err
		}
		if !found {
			return errTxNotAdoptableNow
		}
		if reverted, err := isReverted(); err != nil {
			return err
		} else if reverted {
			return errTxNotAdoptableForever
		}
	}

	checkpoint := f.runtime.State().NewCheckpoint()
	receipt, _, err := f.runtime.ExecuteTransaction(tx)
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
	if f.packer.proposer != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
		return nil, nil, nil, errors.New("private key mismatch")
	}

	if err := f.traverser.Error(); err != nil {
		return nil, nil, nil, err
	}

	stage := f.runtime.State().Stage()
	stateRoot, err := stage.Hash()
	if err != nil {
		return nil, nil, nil, err
	}

	builder := new(block.Builder).
		Beneficiary(f.packer.beneficiary).
		GasLimit(f.runtime.BlockGasLimit()).
		ParentID(f.parentHeader.ID()).
		Timestamp(f.runtime.BlockTime()).
		TotalScore(f.totalScore).
		GasUsed(f.gasUsed).
		ReceiptsRoot(f.receipts.RootHash()).
		StateRoot(stateRoot)
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
