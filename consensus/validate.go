package consensus

import (
	"fmt"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func (c *Consensus) validateBlockHeader(header *block.Header, parent *block.Header, nowTimestamp uint64) error {

	if header.Timestamp() <= parent.Timestamp() {
		return consensusError(fmt.Sprintf("block timestamp behind parents: parent %v, current %v", parent.Timestamp(), header.Timestamp()))
	}
	if (header.Timestamp()-parent.Timestamp())%thor.BlockInterval != 0 {
		return consensusError(fmt.Sprintf("block interval not rounded: parent %v, current %v", parent.Timestamp(), header.Timestamp()))
	}

	if header.Timestamp() > nowTimestamp+thor.BlockInterval {
		return errFutureBlock
	}

	if !block.GasLimit(header.GasLimit()).IsValid(parent.GasLimit()) {
		return consensusError(fmt.Sprintf("block gas limit invalid: parent %v, current %v", parent.GasLimit(), header.GasLimit()))
	}

	if header.GasUsed() > header.GasLimit() {
		return consensusError(fmt.Sprintf("block gas used exceeds limit: limit %v, used %v", header.GasLimit(), header.GasUsed()))
	}

	if header.TotalScore() <= parent.TotalScore() {
		return consensusError(fmt.Sprintf("block total score invalid: parent %v, current %v", parent.TotalScore(), header.TotalScore()))
	}
	return nil
}

func (c *Consensus) validateProposer(header *block.Header, parent *block.Header, st *state.State) error {
	signer, err := header.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}

	authority := builtin.Authority.Native(st)
	endorsement := builtin.Params.Native(st).Get(thor.KeyProposerEndorsement)

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
		return consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
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
		return consensusError(fmt.Sprintf("block txs root mismatch: want %v, have %v", header.TxsRoot(), txs.RootHash()))
	}

	for _, tx := range txs {
		switch {
		case tx.ChainTag() != c.chain.Tag():
			return consensusError(fmt.Sprintf("tx chain tag mismatch: want %v, have %v", c.chain.Tag(), tx.ChainTag()))
		case header.Number() < tx.BlockRef().Number():
			return consensusError(fmt.Sprintf("tx ref future block: ref %v, current %v", tx.BlockRef().Number(), header.Number()))
		case tx.IsExpired(header.Number()):
			return consensusError(fmt.Sprintf("tx expired: ref %v, current %v, expiration %v", tx.BlockRef().Number(), header.Number(), tx.Expiration()))
		case tx.HasReservedFields():
			return consensusError(fmt.Sprintf("tx reserved fields not empty"))
		}
	}

	return nil
}
