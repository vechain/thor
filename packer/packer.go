package packer

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
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

	blockIDGetter := chain.NewBlockIDGetter(p.chain, parent.ID())

	// the runtime for PoA always use parent block env
	poaRT := runtime.New(state, thor.Address{}, parent.Number(), parent.Timestamp(), parent.GasLimit(), blockIDGetter.GetID)
	targetTime, score, err := Schedule(poaRT, p.proposer, now)
	if err != nil {
		return 0, nil, err
	}

	return targetTime, func(txIter TxIterator) (*block.Block, tx.Receipts, error) {

		var gasLimit uint64
		if p.targetGasLimit != 0 {
			gasLimit = thor.GasLimit(p.targetGasLimit).Qualify(parent.GasLimit())
		} else {
			gasLimit = parent.GasLimit()
		}

		builder := new(block.Builder).
			Beneficiary(p.beneficiary).
			GasLimit(gasLimit).
			ParentID(parent.ID()).
			Timestamp(targetTime).
			TotalScore(parent.TotalScore() + score)

		rt := runtime.New(state, p.beneficiary, parent.Number()+1, targetTime, gasLimit, blockIDGetter.GetID)

		receipts, err := p.pack(builder, rt, parent, txIter)
		if err != nil {
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

	totalReward := &big.Int{}

	processed := make(map[thor.Hash]interface{})

	affectedAddresses := make(map[thor.Address]interface{})
	createdContracts := make(map[thor.Address]thor.Address) // contract addr -> master

	rewardRatio := builtin.Params.Get(rt.State(), thor.KeyRewardRatio)

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

		txSigner, err := tx.Signer()
		if err != nil {
			txIter.OnProcessed(tx.ID(), err)
			continue
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

		delay, err := MeasureTxDelay(tx.BlockRef(), parent, p.chain)
		if err != nil {
			return nil, err
		}

		cp := rt.State().NewCheckpoint()
		receipt, vmouts, err := rt.ExecuteTransaction(tx)
		if err != nil {
			// skip and revert state
			rt.State().RevertTo(cp)
			txIter.OnProcessed(tx.ID(), err)
			continue
		}

		for _, vmout := range vmouts {
			for _, addr := range vmout.AffectedAddresses {
				affectedAddresses[addr] = nil
			}
			for _, addr := range vmout.CreatedContracts {
				createdContracts[addr] = txSigner
			}
		}

		receipts = append(receipts, receipt)
		totalGasUsed += receipt.GasUsed

		reward := CalcReward(tx, receipt.GasUsed, rewardRatio, rt.BlockTime(), delay)
		totalReward.Add(totalReward, reward)

		processed[tx.ID()] = nil
		builder.Transaction(tx)
		txIter.OnProcessed(tx.ID(), nil)
	}

	builder.GasUsed(totalGasUsed)

	builtin.Energy.AddBalance(rt.State(), rt.BlockTime(), p.beneficiary, totalReward)

	for addr := range affectedAddresses {
		// touch
		builtin.Energy.AddBalance(rt.State(), rt.BlockTime(), addr, &big.Int{})
	}

	for addr, master := range createdContracts {
		builtin.Energy.SetContractMaster(rt.State(), addr, master)
	}

	return receipts, nil
}
