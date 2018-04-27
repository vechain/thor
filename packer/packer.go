package packer

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
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
type Commit func(privateKey *ecdsa.PrivateKey) (*block.Block, tx.Receipts, [][]tx.Transfers, error)

// Prepare calculates the time to pack and do necessary things before pack.
func (p *Packer) Prepare(parent *block.Header, nowTimestamp uint64) (
	uint64, // target time
	Adopt,
	Commit,
	error) {

	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "state")
	}

	targetTime, score, err := p.schedule(state, parent, nowTimestamp)
	if err != nil {
		return 0, nil, nil, err
	}

	var gasLimit uint64
	if p.targetGasLimit != 0 {
		gasLimit = block.GasLimit(p.targetGasLimit).Qualify(parent.GasLimit())
	} else {
		gasLimit = parent.GasLimit()
	}

	var (
		receipts       tx.Receipts
		blockTransfers [][]tx.Transfers
		totalGasUsed   uint64
		processedTxs   = make(map[thor.Bytes32]bool) // txID -> reverted
		traverser      = p.chain.NewTraverser(parent.ID())
		rt             = runtime.New(state, p.beneficiary, parent.Number()+1, targetTime, gasLimit, func(num uint32) thor.Bytes32 {
			return traverser.Get(num).ID()
		})

		builder = new(block.Builder).
			Beneficiary(p.beneficiary).
			GasLimit(gasLimit).
			ParentID(parent.ID()).
			Timestamp(targetTime).
			TotalScore(parent.TotalScore() + score)
	)

	return targetTime,
		func(tx *tx.Transaction) error {
			switch {
			case tx.ChainTag() != p.chain.Tag():
				return badTxError{"chain tag mismatch"}
			case tx.HasReservedFields():
				return badTxError{"reserved fields not empty"}
			case parent.Number()+1 < tx.BlockRef().Number():
				return errTxNotAdoptableNow
			case tx.IsExpired(parent.Number() + 1):
				return badTxError{"expired"}
			case totalGasUsed+tx.Gas() > gasLimit:
				// gasUsed < 90% gas limit
				if float64(totalGasUsed)/float64(gasLimit) < 0.9 {
					// try to find a lower gas tx
					return errTxNotAdoptableNow
				}
				return errGasLimitReached
			}
			// check if tx already there
			if found, _, err := consensus.FindTransaction(p.chain, parent.ID(), processedTxs, tx.ID()); err != nil {
				return err
			} else if found {
				return errKnownTx
			}

			if dependsOn := tx.DependsOn(); dependsOn != nil {
				// check if deps exists
				found, isReverted, err := consensus.FindTransaction(p.chain, parent.ID(), processedTxs, *dependsOn)
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

			chkpt := state.NewCheckpoint()
			receipt, txTransfers, _, err := rt.ExecuteTransaction(tx)
			if err != nil {
				// skip and revert state
				state.RevertTo(chkpt)
				return badTxError{err.Error()}
			}
			processedTxs[tx.ID()] = receipt.Reverted
			totalGasUsed += receipt.GasUsed
			receipts = append(receipts, receipt)
			blockTransfers = append(blockTransfers, txTransfers)
			builder.Transaction(tx)
			return nil
		},
		func(privateKey *ecdsa.PrivateKey) (*block.Block, tx.Receipts, [][]tx.Transfers, error) {
			if p.proposer != thor.Address(crypto.PubkeyToAddress(privateKey.PublicKey)) {
				return nil, nil, nil, errors.New("private key mismatch")
			}

			if err := traverser.Error(); err != nil {
				return nil, nil, nil, err
			}

			stateRoot, err := state.Stage().Commit()
			if err != nil {
				return nil, nil, nil, err
			}

			newBlock := builder.
				GasUsed(totalGasUsed).
				ReceiptsRoot(receipts.RootHash()).
				StateRoot(stateRoot).Build()

			sig, err := crypto.Sign(newBlock.Header().SigningHash().Bytes(), privateKey)
			if err != nil {
				return nil, nil, nil, err
			}
			return newBlock.WithSignature(sig), receipts, blockTransfers, nil
		}, nil
}

func (p *Packer) schedule(state *state.State, parent *block.Header, nowTimestamp uint64) (
	uint64, // when
	uint64, // score
	error,
) {
	endorsement := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
	authority := builtin.Authority.Native(state)

	candidates := authority.Candidates()
	proposers := make([]poa.Proposer, 0, len(candidates))
	for _, c := range candidates {
		if state.GetBalance(c.Endorsor).Cmp(endorsement) >= 0 {
			proposers = append(proposers, poa.Proposer{
				Address: c.Signer,
				Active:  c.Active,
			})
		}
	}

	// calc the time when it's turn to produce block
	sched, err := poa.NewScheduler(p.proposer, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		return 0, 0, err
	}

	newBlockTime := sched.Schedule(nowTimestamp)
	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		authority.Update(u.Address, u.Active)
	}

	return newBlockTime, score, nil
}

// SetTargetGasLimit set target gas limit, the Packer will adjust block gas limit close to
// it as it can.
func (p *Packer) SetTargetGasLimit(gl uint64) {
	p.targetGasLimit = gl
}
