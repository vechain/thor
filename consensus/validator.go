// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

type blockMetaReader interface {
	ParentID() thor.Bytes32
	Timestamp() uint64
	GasLimit() uint64
	Signer() (thor.Address, error)
}

func (c *Consensus) validate(
	state *state.State,
	block *block.Block,
	parentHeader *block.Header,
	nowTimestamp uint64,
) (*state.Stage, tx.Receipts, error) {
	header := block.Header()

	if err := c.validateBlockHeader(header, parentHeader, nowTimestamp); err != nil {
		return nil, nil, err
	}

	candidates, _, err := c.validateProposer(header, parentHeader, state)
	if err != nil {
		return nil, nil, err
	}

	if err := c.validateBlockBody(block, parentHeader); err != nil {
		return nil, nil, err
	}

	stage, receipts, err := c.verifyBlock(block, state)
	if err != nil {
		return nil, nil, err
	}

	hasAuthorityEvent := func() bool {
		for _, r := range receipts {
			for _, o := range r.Outputs {
				for _, ev := range o.Events {
					if ev.Address == builtin.Authority.Address {
						return true
					}
				}
			}
		}
		return false
	}()

	// if no event emitted from Authority contract, it's believed that the candidates list not changed
	if !hasAuthorityEvent {

		// if no endorsor related transfer, or no event emitted from Params contract, the proposers list
		// can be reused
		hasEndorsorEvent := func() bool {
			for _, r := range receipts {
				for _, o := range r.Outputs {
					for _, ev := range o.Events {
						if ev.Address == builtin.Params.Address {
							return true
						}
					}
					for _, t := range o.Transfers {
						if candidates.IsEndorsor(t.Sender) || candidates.IsEndorsor(t.Recipient) {
							return true
						}
					}
				}
			}
			return false
		}()

		if hasEndorsorEvent {
			candidates.InvalidateCache()
		}
		c.candidatesCache.Add(header.ID(), candidates)
	}
	return stage, receipts, nil
}

func (c *Consensus) validateBlockMeta(header blockMetaReader, parent *block.Header, nowTimestamp uint64) error {
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
	return nil
}

func (c *Consensus) validateBlockHeader(header *block.Header, parent *block.Header, nowTimestamp uint64) error {
	if err := c.validateBlockMeta(header, parent, nowTimestamp); err != nil {
		return err
	}
	if header.GasUsed() > header.GasLimit() {
		return consensusError(fmt.Sprintf("block gas used exceeds limit: limit %v, used %v", header.GasLimit(), header.GasUsed()))
	}
	if header.TotalScore() <= parent.TotalScore() {
		return consensusError(fmt.Sprintf("block total score invalid: parent %v, current %v", parent.TotalScore(), header.TotalScore()))
	}
	if header.TotalBackersCount() < parent.TotalBackersCount() {
		return consensusError(fmt.Sprintf("block total backer count invalid: parent %v, current %v", parent.TotalBackersCount(), header.TotalBackersCount()))
	}

	return nil
}

func (c *Consensus) validateProposer(header blockMetaReader, parent *block.Header, st *state.State) (*poa.Candidates, uint64, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, 0, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}

	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	if entry, ok := c.candidatesCache.Get(parent.ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		list, err := authority.AllCandidates()
		if err != nil {
			return nil, 0, err
		}
		candidates = poa.NewCandidates(list)
	}

	proposers, err := candidates.Pick(st)
	if err != nil {
		return nil, 0, err
	}

	seed, err := c.seeder.Generate(parent)
	if err != nil {
		return nil, 0, err
	}

	sched, err := poa.NewScheduler(signer, proposers, parent.Number(), parent.Timestamp(), seed)
	if err != nil {
		return nil, 0, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return nil, 0, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}

	updates, score := sched.Updates(header.Timestamp())

	if h, ok := header.(*block.Header); ok == true {
		if parent.TotalScore()+score != h.TotalScore() {
			return nil, 0, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, h.TotalScore()))
		}
		authority := builtin.Authority.Native(st)
		for _, u := range updates {
			if _, err := authority.Update(u.Address, u.Active); err != nil {
				return nil, 0, err
			}
			if !candidates.Update(u.Address, u.Active) {
				// should never happen
				panic("something wrong with candidates list")
			}
		}
	}

	return candidates, score, nil
}

func (c *Consensus) validateBackers(blk *block.Block, parent *block.Header, candidates *poa.Candidates, state *state.State) error {
	header := blk.Header()

	if header.Number() >= c.forkConfig.VIP193 {
		backers := blk.Backers()

		totalBackers := uint64(len(backers)) + parent.TotalBackersCount()
		if totalBackers != header.TotalBackersCount() {
			return consensusError(fmt.Sprintf("block total backers count invalid: want %v, have %v", totalBackers, header.TotalBackersCount()))
		}
		if header.BackersRoot() != backers.RootHash() {
			return consensusError(fmt.Sprintf("block backers root mismatch: want %v, have %v", header.BackersRoot(), backers.RootHash()))
		}
		if len(backers) > 0 {
			proposers, err := candidates.Pick(state)
			if err != nil {
				return err
			}
			all := make(map[thor.Address]bool, len(proposers))
			for _, p := range proposers {
				all[p.Address] = true
			}

			alpha := header.Proposal().Hash().Bytes()
			for _, approval := range backers {
				signer, err := approval.Signer()
				if err != nil {
					return consensusError(fmt.Sprintf("block approval's signer unavailable: %v", err))
				}
				if all[signer] == false {
					return consensusError(fmt.Sprintf("backer: %v not in power", signer))
				}
				beta, err := approval.Validate(alpha)
				if err != nil {
					return consensusError(fmt.Sprintf("failed to verify backer's approval %v", err))
				}
				isBacker := poa.EvaluateVRF(beta)
				if isBacker == false {
					return consensusError(fmt.Sprintf("signer is not qualified to be a backer: %v", signer))
				}
			}
		}
	}
	return nil
}

func (c *Consensus) validateBlockBody(blk *block.Block, parent *block.Header) error {
	header := blk.Header()
	txs := blk.Transactions()
	if header.TxsRoot() != txs.RootHash() {
		return consensusError(fmt.Sprintf("block txs root mismatch: want %v, have %v", header.TxsRoot(), txs.RootHash()))
	}

	for _, tx := range txs {
		origin, err := tx.Origin()
		if err != nil {
			return consensusError(fmt.Sprintf("tx signer unavailable: %v", err))
		}

		if header.Number() >= c.forkConfig.BLOCKLIST && thor.IsOriginBlocked(origin) {
			return consensusError(fmt.Sprintf("tx origin blocked got packed: %v", origin))
		}

		switch {
		case tx.ChainTag() != c.repo.ChainTag():
			return consensusError(fmt.Sprintf("tx chain tag mismatch: want %v, have %v", c.repo.ChainTag(), tx.ChainTag()))
		case header.Number() < tx.BlockRef().Number():
			return consensusError(fmt.Sprintf("tx ref future block: ref %v, current %v", tx.BlockRef().Number(), header.Number()))
		case tx.IsExpired(header.Number()):
			return consensusError(fmt.Sprintf("tx expired: ref %v, current %v, expiration %v", tx.BlockRef().Number(), header.Number(), tx.Expiration()))
		}

		if err := tx.TestFeatures(header.TxsFeatures()); err != nil {
			return consensusError("invalid tx: " + err.Error())
		}
	}

	return nil
}

func (c *Consensus) verifyBlock(blk *block.Block, state *state.State) (*state.Stage, tx.Receipts, error) {
	var totalGasUsed uint64
	txs := blk.Transactions()
	receipts := make(tx.Receipts, 0, len(txs))
	processedTxs := make(map[thor.Bytes32]bool)
	header := blk.Header()
	signer, _ := header.Signer()
	chain := c.repo.NewChain(header.ParentID())

	rt := runtime.New(
		chain,
		state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
		},
		c.forkConfig)

	findTx := func(txID thor.Bytes32) (found bool, reverted bool, err error) {
		if reverted, ok := processedTxs[txID]; ok {
			return true, reverted, nil
		}

		meta, err := chain.GetTransactionMeta(txID)
		if err != nil {
			if chain.IsNotFound(err) {
				return false, false, nil
			}
			return false, false, err
		}
		return true, meta.Reverted, nil
	}

	for _, tx := range txs {
		// check if tx existed
		if found, _, err := findTx(tx.ID()); err != nil {
			return nil, nil, err
		} else if found {
			return nil, nil, consensusError("tx already exists")
		}

		// check depended tx
		if dep := tx.DependsOn(); dep != nil {
			found, reverted, err := findTx(*dep)
			if err != nil {
				return nil, nil, err
			}
			if !found {
				return nil, nil, consensusError("tx dep broken")
			}

			if reverted {
				return nil, nil, consensusError("tx dep reverted")
			}
		}

		receipt, err := rt.ExecuteTransaction(tx)
		if err != nil {
			return nil, nil, err
		}

		totalGasUsed += receipt.GasUsed
		receipts = append(receipts, receipt)
		processedTxs[tx.ID()] = receipt.Reverted
	}

	if header.GasUsed() != totalGasUsed {
		return nil, nil, consensusError(fmt.Sprintf("block gas used mismatch: want %v, have %v", header.GasUsed(), totalGasUsed))
	}

	receiptsRoot := receipts.RootHash()
	if header.ReceiptsRoot() != receiptsRoot {
		if c.correctReceiptsRoots[header.ID().String()] != receiptsRoot.String() {
			return nil, nil, consensusError(fmt.Sprintf("block receipts root mismatch: want %v, have %v", header.ReceiptsRoot(), receiptsRoot))
		}
	}

	stage, err := state.Stage()
	if err != nil {
		return nil, nil, err
	}
	stateRoot := stage.Hash()

	if blk.Header().StateRoot() != stateRoot {
		return nil, nil, consensusError(fmt.Sprintf("block state root mismatch: want %v, have %v", header.StateRoot(), stateRoot))
	}

	return stage, receipts, nil
}
