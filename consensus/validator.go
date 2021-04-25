// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

var emptyRoot = thor.Blake2b(rlp.EmptyString) // This is the known root hash of an empty trie.

func (c *Consensus) validate(
	state *state.State,
	block *block.Block,
	parent *block.Block,
	nowTimestamp uint64,
) (*state.Stage, tx.Receipts, mclock.AbsTime, error) {
	header := block.Header()

	if err := c.validateBlockHeader(header, parent.Header(), nowTimestamp); err != nil {
		return nil, nil, 0, err
	}

	start := mclock.Now()
	candidates, proposers, maxBlockProposers, err := c.validateProposer(header, parent, state)
	if err != nil {
		return nil, nil, 0, err
	}
	et := mclock.Now() - start

	if err := c.validateBlockBody(block, parent.Header(), proposers, maxBlockProposers); err != nil {
		return nil, nil, 0, err
	}

	stage, receipts, err := c.verifyBlock(block, state)
	if err != nil {
		return nil, nil, 0, err
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
	return stage, receipts, et, nil
}

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

	if header.Number() < c.forkConfig.VIP193 {
		if len(header.Signature()) != 65 {
			return consensusError("invalid signature length")
		}
		if header.BackerSignaturesRoot() != emptyRoot {
			return consensusError("invalid block header: backer signature root should be empty root before fork VIP193")
		}
		if header.TotalQuality() != 0 {
			return consensusError("invalid block header: total quality should be 0 before fork VIP193")
		}
		if len(header.Alpha()) != 0 {
			return consensusError("invalid block header: alpha should be nil before fork VIP193")
		}
	} else {
		if len(header.Signature()) != 146 {
			return consensusError("invalid signature length")
		}
		if header.TotalQuality() < parent.TotalQuality() {
			return consensusError(fmt.Sprintf("block quality invalid: parent %v, current %v", parent.TotalQuality(), header.TotalQuality()))
		}
	}

	return nil
}

func (c *Consensus) validateProposer(header *block.Header, parent *block.Block, st *state.State) (*poa.Candidates, []poa.Proposer, uint64, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, nil, 0, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}

	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	if entry, ok := c.candidatesCache.Get(parent.Header().ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		list, err := authority.AllCandidates()
		if err != nil {
			return nil, nil, 0, err
		}
		candidates = poa.NewCandidates(list)
	}

	mbp, err := builtin.Params.Native(st).Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return nil, nil, 0, err
	}
	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 {
		maxBlockProposers = thor.InitialMaxBlockProposers
	}

	proposers, err := candidates.Pick(st, maxBlockProposers)
	if err != nil {
		return nil, nil, 0, err
	}

	var sched poa.Scheduler
	if header.Number() >= c.forkConfig.VIP193 {
		var seed thor.Bytes32
		seed, err = c.seeder.Generate(header.ParentID())
		if err != nil {
			return nil, nil, 0, err
		}
		sched, err = poa.NewSchedulerV2(signer, proposers, parent, seed.Bytes())
	} else {
		sched, err = poa.NewSchedulerV1(signer, proposers, parent.Header().Number(), parent.Header().Timestamp())
	}
	if err != nil {
		return nil, nil, 0, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return nil, nil, 0, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.Header().TotalScore()+score != header.TotalScore() {
		return nil, nil, 0, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.Header().TotalScore()+score, header.TotalScore()))
	}

	for _, u := range updates {
		if _, err := authority.Update(u.Address, u.Active); err != nil {
			return nil, nil, 0, err
		}
		if !candidates.Update(u.Address, u.Active) {
			// should never happen
			panic("something wrong with candidates list")
		}
	}

	return candidates, proposers, maxBlockProposers, nil
}

func (c *Consensus) validateBlockBody(blk *block.Block, parent *block.Header, proposers []poa.Proposer, maxBlockProposers uint64) error {
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

	bss := blk.BackerSignatures()
	if header.Number() < c.forkConfig.VIP193 {
		if len(bss) != 0 {
			return consensusError("invalid block: backer signatures should be empty before fork VIP193")
		}
	} else {
		seed, _ := c.seeder.Generate(header.ParentID())
		alpha := append([]byte(nil), seed.Bytes()...)
		alpha = append(alpha, header.ParentID().Bytes()[:4]...)

		if !bytes.Equal(header.Alpha(), alpha) {
			return consensusError(fmt.Sprintf("alpha mismatch: want %s, have %s", hexutil.Bytes(alpha), hexutil.Bytes(header.Alpha())))
		}

		if _, err := header.Beta(); err != nil {
			return consensusError("failed to verify VRF in header: " + err.Error())
		}

		if header.BackerSignaturesRoot() != bss.RootHash() {
			return consensusError(fmt.Sprintf("block backers root mismatch: want %v, have %v", header.BackerSignaturesRoot(), bss.RootHash()))
		}

		totalQuality := parent.TotalQuality()
		if len(bss) >= thor.HeavyBlockRequirement {
			totalQuality++
		}
		if totalQuality != header.TotalQuality() {
			return consensusError(fmt.Sprintf("block total quality mismatch: want %v, have %v", header.TotalQuality(), totalQuality))
		}

		if len(bss) > 0 {
			proposer, _ := header.Signer()
			getBacker := func(addr thor.Address) *poa.Proposer {
				for _, p := range proposers {
					if p.Address == addr {
						return &poa.Proposer{
							Address: p.Address,
							Active:  p.Active,
						}
					}
				}
				return nil
			}

			backers, betas, err := blk.Committee()
			if err != nil {
				return consensusError(fmt.Sprintf("failed to get block committee: %v", err))
			}

			prev := []byte{}
			for i, addr := range backers {
				if b := getBacker(addr); b == nil {
					return consensusError(fmt.Sprintf("backer: %v is not an authority", addr))
				}

				if addr == proposer {
					return consensusError("block signer cannot back itself")
				}

				if bytes.Compare(prev, betas[i]) > 0 {
					return consensusError("backer signatures are not in ascending order(by beta)")
				}

				prev = betas[i]
				if !poa.EvaluateVRF(betas[i], maxBlockProposers) {
					return consensusError(fmt.Sprintf("invalid proof from %v", addr))
				}
			}
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

	if header.StateRoot() != stateRoot {
		return nil, nil, consensusError(fmt.Sprintf("block state root mismatch: want %v, have %v", header.StateRoot(), stateRoot))
	}

	return stage, receipts, nil
}
