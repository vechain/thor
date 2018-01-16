package proposer

import (
	"math"
	"math/big"
	"time"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type Proposer struct {
	chain *chain.Chain

	signer      thor.Address
	beneficiary thor.Address

	targetGasLimit uint64

	signing *cry.Signing
}

func New(chain *chain.Chain, signing *cry.Signing) *Proposer {
	return &Proposer{
		chain,
		thor.Address{},
		thor.Address{},
		0,
		signing,
	}
}

func (p *Proposer) Schedule(state *state.State, parent *block.Header) (
	timestamp uint64,
	score uint64,
	err error,
) {
	hashGetter := chain.NewHashGetter(p.chain, parent.Hash())
	rt := runtime.New(state, thor.Address{}, 0, 0, 0, hashGetter.GetHash)

	// invoke `Authority.proposers()` to get current proposers whitelist
	out := rt.Execute(&tx.Clause{
		To:    &contracts.Authority.Address,
		Value: &big.Int{},
		Data:  contracts.Authority.PackProposers(),
	}, 0, math.MaxUint64, thor.Address{}, &big.Int{}, thor.Hash{})

	if out.VMErr != nil {
		return 0, 0, errors.Wrap(out.VMErr, "vm error")
	}

	proposers := contracts.Authority.UnpackProposers(out.Value)

	// calc the time when it's turn to produce block
	targetTime, updates, err := schedule.New(proposers, parent.Number(), parent.Timestamp()).
		Timing(p.signer, uint64(time.Now().Unix()))
	if err != nil {
		return 0, 0, err
	}

	// update proposers' status
	out = rt.Execute(&tx.Clause{
		To:    &contracts.Authority.Address,
		Value: &big.Int{},
		Data:  contracts.Authority.PackUpdate(updates),
	}, 0, math.MaxUint64, contracts.Authority.Address, &big.Int{}, thor.Hash{})

	if out.VMErr != nil {
		return 0, 0, errors.Wrap(out.VMErr, "vm")
	}

	if err := state.Error(); err != nil {
		return 0, 0, errors.Wrap(err, "state")
	}
	if err := hashGetter.Error(); err != nil {
		return 0, 0, errors.Wrap(err, "hash getter")
	}

	return targetTime, calcScore(proposers, updates), nil
}

func (p *Proposer) precheckTx(tx *tx.Transaction, parentHash thor.Hash, timestamp uint64) (bool, error) {
	if tx.TimeBarrier() > timestamp {
		return false, nil
	}

	_, err := p.chain.LookupTransaction(parentHash, tx.Hash())
	if err != nil {
		if !p.chain.IsNotFound(err) {
			return false, err
		}
	} else {
		return false, nil
	}

	return true, nil
}

func (p *Proposer) SetTargetGasLimit(gl uint64) {
	p.targetGasLimit = gl
}

func (p *Proposer) Propose(
	state *state.State,
	parent *block.Header,
	timestamp uint64,
	score uint64,
	txFeed TxFeed) (*block.Block, tx.Receipts, error) {

	var gasLimit uint64
	if p.targetGasLimit != 0 {
		gasLimit = thor.GasLimit(p.targetGasLimit).Qualify(parent.GasLimit())
	} else {
		gasLimit = parent.GasLimit()
	}

	builder := new(block.Builder).
		ParentHash(parent.Hash()).
		Beneficiary(p.beneficiary).
		GasLimit(gasLimit)

	hashGetter := chain.NewHashGetter(p.chain, parent.Hash())
	rt := runtime.New(
		state,
		p.beneficiary,
		parent.Number()+1,
		timestamp,
		gasLimit,
		hashGetter.GetHash)

	var receipts tx.Receipts
	var totalGasUsed uint64
	for {
		tx := txFeed.Next()
		if tx == nil {
			break
		}

		if tx.Gas() > gasLimit-totalGasUsed {
			break
		}

		if ok, err := p.precheckTx(tx, parent.Hash(), timestamp); err != nil {
			return nil, nil, err
		} else if !ok {
			continue
		}

		cp := state.NewCheckpoint()
		receipt, vmouts, err := rt.ExecuteTransaction(tx, p.signing)
		_ = vmouts
		if err != nil {
			// skip and revert state
			state.RevertTo(cp)
			continue
		}
		if err := state.Error(); err != nil {
			return nil, nil, err
		}
		if err := hashGetter.Error(); err != nil {
			return nil, nil, err
		}

		receipts = append(receipts, receipt)
		totalGasUsed += receipt.GasUsed

		builder.Transaction(tx)
	}

	stateRoot, err := state.Stage().Commit()
	if err != nil {
		return nil, nil, err
	}

	return builder.
		GasUsed(totalGasUsed).
		ReceiptsRoot(receipts.RootHash()).
		StateRoot(stateRoot).Build(), receipts, nil

}

func calcScore(all []schedule.Proposer, updates []schedule.Proposer) uint64 {
	absentee := make(map[thor.Address]interface{})
	for _, p := range all {
		if p.IsAbsent() {
			absentee[p.Address] = nil
		}
	}

	for _, p := range updates {
		if p.IsAbsent() {
			absentee[p.Address] = nil
		} else {
			delete(absentee, p.Address)
		}
	}
	return uint64(len(all) - len(absentee))
}
