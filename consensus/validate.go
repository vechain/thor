package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func (c *Consensus) validateBlockHeader(header *block.Header, parent *block.Header, nowTimestamp uint64) error {

	if header.Timestamp() <= parent.Timestamp() {
		return errors.New("block timestamp too small")
	}
	if (header.Timestamp()-parent.Timestamp())%thor.BlockInterval != 0 {
		return errors.New("invalid block interval")
	}

	if header.Timestamp() > nowTimestamp+thor.BlockInterval {
		return errFutureBlock
	}

	if !block.GasLimit(header.GasLimit()).IsValid(parent.GasLimit()) {
		return errors.New("invalid block gas limit")
	}

	if header.GasUsed() > header.GasLimit() {
		return errors.New("block gas used exceeds limit")
	}

	if header.TotalScore() <= parent.TotalScore() {
		return errors.New("block total score too small")
	}
	return nil
}

func (c *Consensus) validateProposer(header *block.Header, parent *block.Header, st *state.State) error {
	signer, err := header.Signer()
	if err != nil {
		return errors.Wrap(err, "invalid block signer")
	}

	authority := builtin.Authority.WithState(st)
	endorsement := builtin.Params.WithState(st).Get(thor.KeyProposerEndorsement)

	candidates := authority.Candidates()
	proposers := make([]poa.Proposer, 0, len(candidates))
	for _, c := range candidates {
		if st.GetBalance(c.Endorsor).Cmp(endorsement) >= 0 {
			proposers = append(proposers, poa.Proposer{
				Address: c.Signer,
				Active:  c.Active,
			})
		}
	}

	sched, err := poa.NewScheduler(signer, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		return err
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return errors.New("block timestamp not scheduled")
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return errors.New("incorrect block total score")
	}

	for _, proposer := range updates {
		authority.Update(proposer.Address, proposer.Active)
	}
	return nil
}

func (c *Consensus) validateBlockBody(blk *block.Block) error {
	header := blk.Header()
	txs := blk.Transactions()
	if header.TxsRoot() != txs.RootHash() {
		return errors.New("incorrect block txs root")
	}

	if len(txs) == 0 {
		return nil
	}

	passedTxs := make(map[thor.Bytes32]struct{})
	hasTx := func(txID thor.Bytes32) (bool, error) {
		if _, ok := passedTxs[txID]; ok {
			return true, nil
		}
		_, err := c.chain.LookupTransaction(header.ParentID(), txID)
		if err != nil {
			if c.chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	for _, tx := range txs {
		switch {
		case tx.ChainTag() != c.chain.Tag():
			return errors.New("bad tx: chain tag mismatch")
		case tx.BlockRef().Number() >= header.Number():
			return errors.New("bad tx: invalid block ref")
		case tx.HasReservedFields():
			return errors.New("bad tx: reserved fields not empty")
		}
		// check if tx already appeared in old blocks, or passed map
		if found, err := hasTx(tx.ID()); err != nil {
			return err
		} else if found {
			return errors.New("bad tx: duplicated tx")
		}

		// check depended tx
		if dep := tx.DependsOn(); dep != nil {
			if found, err := hasTx(*dep); err != nil {
				return err
			} else if !found {
				return errors.New("bad tx: dep not found")
			}
		}
		passedTxs[tx.ID()] = struct{}{}
	}
	return nil
}
