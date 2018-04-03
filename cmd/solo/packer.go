package main

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	errGasLimitReached   = errors.New("gas limit reached")
	errTxNotAdoptableNow = errors.New("tx not adoptable now")
	errKnownTx           = errors.New("known tx")
)

// SoloPacker to pack txs and build new blocks.
type SoloPacker struct {
	chain          *chain.Chain
	stateCreator   *state.Creator
	proposer       thor.Address
	beneficiary    thor.Address
	targetGasLimit uint64
}

type badTxError struct {
	msg string
}

func (e badTxError) Error() string {
	return "bad tx: " + e.msg
}

// NewSoloPacker create a new SoloPacker instance.
func NewSoloPacker(
	chain *chain.Chain,
	stateCreator *state.Creator,
	proposer thor.Address,
	beneficiary thor.Address) *SoloPacker {

	return &SoloPacker{
		chain,
		stateCreator,
		proposer,
		beneficiary,
		0,
	}
}

// Prepare is for diffing from `Prepare` in package packer, SoloPacker works itself doesn't deal with scheduler, Setup function accept new block timestamp as an param
func (p *SoloPacker) Prepare(parent *block.Header, newBlockTimestamp uint64) (
	packer.Adopt,
	packer.Commit,
	error) {

	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return nil, nil, errors.Wrap(err, "state")
	}

	var score uint64 = 1
	var gasLimit uint64
	if p.targetGasLimit != 0 {
		gasLimit = block.GasLimit(p.targetGasLimit).Qualify(parent.GasLimit())
	} else {
		gasLimit = parent.GasLimit()
	}

	var (
		receipts     tx.Receipts
		totalGasUsed uint64
		processedTxs = make(map[thor.Bytes32]struct{})
		traverser    = p.chain.NewTraverser(parent.ID())
		rt           = runtime.New(state, p.beneficiary, parent.Number()+1, newBlockTimestamp, gasLimit, func(num uint32) thor.Bytes32 {
			return traverser.Get(num).ID()
		})
		findTx  = p.newTxFinder(parent.ID(), processedTxs)
		builder = new(block.Builder).
			Beneficiary(p.beneficiary).
			GasLimit(gasLimit).
			ParentID(parent.ID()).
			Timestamp(newBlockTimestamp).
			TotalScore(parent.TotalScore() + score)
	)

	return func(tx *tx.Transaction) error {
			switch {
			case tx.ChainTag() != p.chain.Tag():
				return badTxError{"chain tag mismatch"}
			case tx.HasReservedFields():
				return badTxError{"reserved fields not empty"}
			case tx.BlockRef().Number() > parent.Number():
				return errTxNotAdoptableNow
			case totalGasUsed+tx.Gas() > gasLimit:
				// gasUsed < 90% gas limit
				if float64(gasLimit-totalGasUsed)/float64(gasLimit) < 0.9 {
					// try to find a lower gas tx
					return errTxNotAdoptableNow
				}
				return errGasLimitReached
			}
			// check if tx already there
			if found, err := findTx(tx.ID()); err != nil {
				return err
			} else if found {
				return errKnownTx
			}
			if dependsOn := tx.DependsOn(); dependsOn != nil {
				// check if deps exists
				if found, err := findTx(*dependsOn); err != nil {
					return err
				} else if !found {
					return errTxNotAdoptableNow
				}
			}

			chkpt := state.NewCheckpoint()
			receipt, _, err := rt.ExecuteTransaction(tx)
			if err != nil {
				// skip and revert state
				state.RevertTo(chkpt)
				return badTxError{err.Error()}
			}
			processedTxs[tx.ID()] = struct{}{}
			totalGasUsed += receipt.GasUsed
			receipts = append(receipts, receipt)
			builder.Transaction(tx)
			return nil
		},
		func(privateKey *ecdsa.PrivateKey) (*block.Block, tx.Receipts, error) {
			if p.proposer != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
				return nil, nil, errors.New("private key mismatch")
			}

			if err := traverser.Error(); err != nil {
				return nil, nil, err
			}

			stateRoot, err := state.Stage().Commit()
			if err != nil {
				return nil, nil, err
			}

			newBlock := builder.
				GasUsed(totalGasUsed).
				ReceiptsRoot(receipts.RootHash()).
				StateRoot(stateRoot).Build()

			sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
			if err != nil {
				return nil, nil, err
			}
			return newBlock.WithSignature(sig), receipts, nil
		}, nil
}

func (p *SoloPacker) newTxFinder(parentBlockID thor.Bytes32, processed map[thor.Bytes32]struct{}) func(txID thor.Bytes32) (bool, error) {
	return func(txID thor.Bytes32) (bool, error) {
		if _, ok := processed[txID]; ok {
			return true, nil
		}
		if _, err := p.chain.LookupTransaction(parentBlockID, txID); err != nil {
			if p.chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
}

// IsGasLimitReached block if full of txs.
func IsGasLimitReached(err error) bool {
	return errors.Cause(err) == errGasLimitReached
}

// IsTxNotAdoptableNow tx can not be adopted now.
func IsTxNotAdoptableNow(err error) bool {
	return errors.Cause(err) == errTxNotAdoptableNow
}

// IsBadTx not a valid tx.
func IsBadTx(err error) bool {
	_, ok := errors.Cause(err).(badTxError)
	return ok
}

// IsKnownTx tx is already adopted, or in the chain.
func IsKnownTx(err error) bool {
	return errors.Cause(err) == errKnownTx
}
