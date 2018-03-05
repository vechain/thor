package packer

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
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
	chain          *chain.Chain
	stateCreator   *state.Creator
	proposer       thor.Address
	beneficiary    thor.Address
	targetGasLimit uint64
}

// New create a new Packer instance.
func New(
	chain *chain.Chain,
	stateCreator *state.Creator,
	proposer thor.Address,
	beneficiary thor.Address) *Packer {

	return &Packer{
		chain,
		stateCreator,
		proposer,
		beneficiary,
		0,
	}
}

// Adopt adopt transaction into new block.
type Adopt func(tx *tx.Transaction) error

// Commit generate new block.
type Commit func(privateKey *ecdsa.PrivateKey) (*block.Block, tx.Receipts, error)

type context struct {
	parent       *block.Header
	receipts     tx.Receipts
	totalGasUsed uint64
	processed    map[thor.Hash]interface{}
	rt           *runtime.Runtime
}

// Prepare calculates the time to pack and do necessary things before pack.
func (p *Packer) Prepare(now uint64) (
	uint64, // target time
	Adopt,
	Commit,
	error) {

	bestBlock, err := p.chain.GetBestBlock()
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "chain")
	}
	parent := bestBlock.Header()
	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "state")
	}

	targetTime, score, err := p.schedule(state, parent, now)
	if err != nil {
		return 0, nil, nil, err
	}

	var gasLimit uint64
	if p.targetGasLimit != 0 {
		gasLimit = block.GasLimit(p.targetGasLimit).Qualify(parent.GasLimit())
	} else {
		gasLimit = parent.GasLimit()
	}

	traverser := p.chain.NewTraverser(parent.ID())
	builder := new(block.Builder).
		Beneficiary(p.beneficiary).
		GasLimit(gasLimit).
		ParentID(parent.ID()).
		Timestamp(targetTime).
		TotalScore(parent.TotalScore() + score)

	ctx := &context{
		parent:    parent,
		processed: make(map[thor.Hash]interface{}),
		rt: runtime.New(state, p.beneficiary, parent.Number()+1, targetTime, gasLimit, func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		}),
	}

	return targetTime,
		func(tx *tx.Transaction) error {
			if err := p.processTx(ctx, tx); err != nil {
				return err
			}
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
				GasUsed(ctx.totalGasUsed).
				ReceiptsRoot(ctx.receipts.RootHash()).
				StateRoot(stateRoot).Build()

			sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
			if err != nil {
				return nil, nil, err
			}
			return newBlock.WithSignature(sig), ctx.receipts, nil
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

func (p *Packer) processTx(ctx *context, tx *tx.Transaction) error {
	switch {
	case tx.ReservedBits() != 0:
		return badTxError{"reserved bits != 0"}
	case tx.ChainTag() != ctx.parent.ChainTag():
		return badTxError{"chain tag mismatch"}
	case tx.BlockRef().Number() > ctx.parent.Number():
		return errTxNotAdoptableNow
	case ctx.totalGasUsed+tx.Gas() > ctx.rt.BlockGasLimit():
		// gasUsed < 90% gas limit
		if float64(ctx.rt.BlockGasLimit()-ctx.totalGasUsed)/float64(ctx.rt.BlockGasLimit()) < 0.9 {
			// try to find a lower gas tx
			return errTxNotAdoptableNow
		}
		return errGasLimitReached
	}

	// check if tx already there
	if found, err := p.txExists(tx.ID(), ctx.parent.ID(), ctx.processed); err != nil {
		return err
	} else if found {
		return errKnownTx
	}

	if dependsOn := tx.DependsOn(); dependsOn != nil {
		// check if deps exists
		if found, err := p.txExists(*dependsOn, ctx.parent.ID(), ctx.processed); err != nil {
			return err
		} else if !found {
			return errTxNotAdoptableNow
		}
	}

	cp := ctx.rt.State().NewCheckpoint()
	receipt, _, err := ctx.rt.ExecuteTransaction(tx)
	if err != nil {
		// skip and revert state
		ctx.rt.State().RevertTo(cp)
		return badTxError{err.Error()}
	}

	ctx.receipts = append(ctx.receipts, receipt)
	ctx.totalGasUsed += receipt.GasUsed

	ctx.processed[tx.ID()] = nil
	return nil
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
