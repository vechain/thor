package packer

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Packer to pack txs and build new blocks.
type Packer struct {
	proposer     thor.Address
	beneficiary  thor.Address
	chain        *chain.Chain
	stateCreator *state.Creator

	targetGasLimit uint64
}

var big2 = big.NewInt(2)

// New create a new Packer instance.
func New(
	proposer thor.Address,
	beneficiary thor.Address,
	chain *chain.Chain,
	stateCreator *state.Creator) *Packer {

	return &Packer{
		proposer,
		beneficiary,
		chain,
		stateCreator,
		0,
	}
}

// PackFn function to do packing things.
type PackFn func(TxIterator) (*block.Block, tx.Receipts, error)

// Prepare calculates the time to pack and do necessary things before pack.
func (p *Packer) Prepare(parent *block.Header, now uint64) (ts uint64, pack PackFn, err error) {
	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return 0, nil, errors.Wrap(err, "state")
	}

	targetTime, score, err := p.schedule(state, parent, now)
	if err != nil {
		return 0, nil, err
	}

	return targetTime, func(txIter TxIterator) (*block.Block, tx.Receipts, error) {

		var gasLimit uint64
		if p.targetGasLimit != 0 {
			gasLimit = block.GasLimit(p.targetGasLimit).Qualify(parent.GasLimit())
		} else {
			gasLimit = parent.GasLimit()
		}

		builder := new(block.Builder).
			Beneficiary(p.beneficiary).
			GasLimit(gasLimit).
			ParentID(parent.ID()).
			Timestamp(targetTime).
			TotalScore(parent.TotalScore() + score)

		traverser := p.chain.NewTraverser(parent.ID())
		rt := runtime.New(state, p.beneficiary, parent.Number()+1, targetTime, gasLimit, func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})

		receipts, err := p.pack(builder, rt, parent, txIter)
		if err != nil {
			return nil, nil, err
		}

		if err := traverser.Error(); err != nil {
			return nil, nil, err
		}

		stateRoot, err := state.Stage().Commit()
		if err != nil {
			return nil, nil, err
		}

		return builder.
			ReceiptsRoot(receipts.RootHash()).
			StateRoot(stateRoot).Build(), receipts, nil
	}, nil
}

func (p *Packer) schedule(state *state.State, parent *block.Header, now uint64) (
	uint64, // when
	uint64, // score
	error,
) {
	// use parent block as env
	rt := runtime.New(state, thor.Address{}, parent.Number(), parent.Timestamp(), parent.GasLimit(), nil /*nil is safe here*/)
	proposers := builtin.Authority.All(rt.State())

	// calc the time when it's turn to produce block
	sched, err := poa.NewScheduler(p.proposer, proposers, rt.BlockNumber(), rt.BlockTime())
	if err != nil {
		return 0, 0, err
	}

	newBlockTime := sched.Schedule(now)

	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		builtin.Authority.Update(rt.State(), u.Address, u.Status)
	}

	return newBlockTime, score, nil
}

func (p *Packer) txExists(txID thor.Hash, parentID thor.Hash, processed map[thor.Hash]interface{}) (bool, error) {
	if _, ok := processed[txID]; ok {
		return true, nil
	}
	_, err := p.chain.LookupTransaction(parentID, txID)
	if err != nil {
		if p.chain.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SetTargetGasLimit set target gas limit, the Packer will adjust block gas limit close to
// it as it can.
func (p *Packer) SetTargetGasLimit(gl uint64) {
	p.targetGasLimit = gl
}

func (p *Packer) pack(
	builder *block.Builder,
	rt *runtime.Runtime,
	parent *block.Header,
	txIter TxIterator) (tx.Receipts, error) {

	var receipts tx.Receipts
	var totalGasUsed uint64

	processed := make(map[thor.Hash]interface{})
	for txIter.HasNext() {
		tx := txIter.Next()

		if tx.ReservedBits() != 0 {
			txIter.OnProcessed(tx.ID(), errors.New("unacceptable tx: reserved bits != 0"))
			continue
		}
		if tx.ChainTag() != parent.ChainTag() {
			txIter.OnProcessed(tx.ID(), errors.New("unacceptable tx: chain tag mismatch"))
			continue
		}

		blockRef := tx.BlockRef()
		if blockRef.Number() > parent.Number() {
			continue
		}

		if totalGasUsed+tx.Gas() > rt.BlockGasLimit() {
			// gasUsed < 90% gas limit
			if float64(rt.BlockGasLimit()-totalGasUsed)/float64(rt.BlockGasLimit()) < 0.9 {
				// try to find a lower gas tx
				continue
			}
			break
		}

		// check if tx already there
		if found, err := p.txExists(tx.ID(), parent.ID(), processed); err != nil {
			return nil, err
		} else if found {
			txIter.OnProcessed(tx.ID(), errors.New("tx found"))
			continue
		}

		if dependsOn := tx.DependsOn(); dependsOn != nil {
			// check if deps exists
			if found, err := p.txExists(*dependsOn, parent.ID(), processed); err != nil {
				return nil, err
			} else if !found {
				continue
			}
		}

		cp := rt.State().NewCheckpoint()
		receipt, _, err := rt.ExecuteTransaction(tx)
		if err != nil {
			// skip and revert state
			rt.State().RevertTo(cp)
			txIter.OnProcessed(tx.ID(), err)
			continue
		}

		receipts = append(receipts, receipt)
		totalGasUsed += receipt.GasUsed

		processed[tx.ID()] = nil
		builder.Transaction(tx)
		txIter.OnProcessed(tx.ID(), nil)
	}

	builder.GasUsed(totalGasUsed)

	return receipts, nil
}
