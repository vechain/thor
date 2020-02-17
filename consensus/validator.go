// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

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

	candidates, err := c.validateProposer(header, parentHeader, state)
	if err != nil {
		return nil, nil, err
	}

	if err := c.validateBlockBody(block); err != nil {
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

func (c *Consensus) validateBlockHeaderVip193(header *block.Header, parentHeader *block.Header) error {
	// reconstruct and validate the block summary
	bs := header.BlockSummary()
	// bs := block.NewBlockSummary(
	// 	header.ParentID(),
	// 	header.TxsRoot(),
	// 	header.Timestamp(),
	// 	header.TotalScore()).WithSignature(header.SigOnBlockSummary())

	if err := c.ValidateBlockSummary(bs, parentHeader, header.Timestamp()); err != nil {
		return err.(consensusError).AddTraceInfo(trHeader)
	}

	// reconstuct and validate endoresements
	sigs := header.SigsOnEndoresment()
	proofs := header.VrfProofs()

	if len(sigs) != int(thor.CommitteeSize) {
		return newConsensusError(trHeader, "invalid number of endoresment signatures",
			[]string{strDataExpected, strDataCurr},
			[]interface{}{int(thor.CommitteeSize), len(sigs)}, "")
	}

	if len(proofs) != int(thor.CommitteeSize) {
		return newConsensusError(trHeader, "invalid number of vrf proofs",
			[]string{strDataExpected, strDataCurr},
			[]interface{}{int(thor.CommitteeSize), len(proofs)}, "")
	}

	eds := header.Endorsements()
	for _, ed := range eds {
		// for i, proof := range proofs {
		// ed := block.NewEndorsement(bs, proof).WithSignature(sigs[i])
		if err := c.ValidateEndorsement(ed, parentHeader, header.Timestamp()); err != nil {
			return err.(consensusError).AddTraceInfo(trHeader)
		}
	}

	return nil
}

// ValidateBlockHeader validates a block header of the CURRENT round
func (c *Consensus) ValidateBlockHeader(header *block.Header, parentHeader *block.Header, nowTimestamp uint64) error {
	if header == nil {
		return newConsensusError(trHeader, "empty header", nil, nil, "")
	}

	// Check if the header is in the current round
	if c.Timestamp(nowTimestamp) != header.Timestamp() {
		return newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataExpected, strDataCurr, strDataNowTime},
			[]interface{}{c.Timestamp(nowTimestamp), header.Timestamp(), nowTimestamp}, "")
	}

	if !block.GasLimit(header.GasLimit()).IsValid(parentHeader.GasLimit()) {
		// return consensusError(fmt.Sprintf("block gas limit invalid: parent %v, current %v", parent.GasLimit(), header.GasLimit()))
		return newConsensusError(trHeader, strErrGasLimit,
			[]string{strDataParent, strDataCurr},
			[]interface{}{parentHeader.GasLimit(), header.GasLimit()}, "")
	}

	if header.GasUsed() > header.GasLimit() {
		// return consensusError(fmt.Sprintf("block gas used exceeds limit: limit %v, used %v", header.GasLimit(), header.GasUsed()))
		return newConsensusError(trHeader, strErrGasExceed,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{header.GasLimit(), header.GasUsed()}, "")
	}

	if header.TotalScore() <= parentHeader.TotalScore() {
		// return consensusError(fmt.Sprintf("block total score invalid: parent %v, current %v", parent.TotalScore(), header.TotalScore()))
		return newConsensusError(trHeader, strErrTotalScore,
			[]string{strDataParent, strDataCurr},
			[]interface{}{parentHeader.TotalScore(), header.TotalScore()}, "")
	}

	return c.validateBlockHeaderVip193(header, parentHeader)
}

func (c *Consensus) validateBlockHeader(header *block.Header, parentHeader *block.Header, nowTimestamp uint64) error {
	if header.Timestamp() <= parentHeader.Timestamp() {
		// return consensusError(fmt.Sprintf("block timestamp behind parents: parent %v, current %v", parent.Timestamp(), header.Timestamp()))
		return newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{parentHeader.Timestamp(), header.Timestamp()}, "")
	}

	if (header.Timestamp()-parentHeader.Timestamp())%thor.BlockInterval != 0 {
		// return consensusError(fmt.Sprintf("block interval not rounded: parent %v, current %v", parent.Timestamp(), header.Timestamp()))
		return newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{parentHeader.Timestamp(), header.Timestamp()}, "")
	}

	if header.Timestamp() > nowTimestamp+thor.BlockInterval {
		return errFutureBlock
	}

	if !block.GasLimit(header.GasLimit()).IsValid(parentHeader.GasLimit()) {
		// return consensusError(fmt.Sprintf("block gas limit invalid: parent %v, current %v", parent.GasLimit(), header.GasLimit()))
		return newConsensusError(trHeader, strErrGasLimit,
			[]string{strDataParent, strDataCurr},
			[]interface{}{parentHeader.GasLimit(), header.GasLimit()}, "")
	}

	if header.GasUsed() > header.GasLimit() {
		// return consensusError(fmt.Sprintf("block gas used exceeds limit: limit %v, used %v", header.GasLimit(), header.GasUsed()))
		return newConsensusError(trHeader, strErrGasExceed,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{header.GasLimit(), header.GasUsed()}, "")
	}

	if header.TotalScore() <= parentHeader.TotalScore() {
		// return consensusError(fmt.Sprintf("block total score invalid: parent %v, current %v", parent.TotalScore(), header.TotalScore()))
		return newConsensusError(trHeader, strErrTotalScore,
			[]string{strDataParent, strDataCurr},
			[]interface{}{parentHeader.TotalScore(), header.TotalScore()}, "")
	}

	return c.validateBlockHeaderVip193(header, parentHeader)
}

func (c *Consensus) validateProposer(header *block.Header, parent *block.Header, st *state.State) (*poa.Candidates, error) {
	signer, err := header.Signer()
	if err != nil {
		// return nil, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
		return nil, newConsensusError(trProposer, strErrSignature, nil, nil, err.Error())
	}

	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	if entry, ok := c.candidatesCache.Get(parent.ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		list, err := authority.AllCandidates()
		if err != nil {
			return nil, err
		}
		candidates = poa.NewCandidates(list)
	}

	proposers, err := candidates.Pick(st)
	if err != nil {
		return nil, err
	}

	sched, err := poa.NewScheduler(signer, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		// return nil, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
		return nil, newConsensusError(trProposer, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{signer}, err.Error())
	}

	if !sched.IsTheTime(header.Timestamp()) {
		// return nil, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
		return nil, newConsensusError(trProposer, strErrTimestampUnsched,
			[]string{strDataTimestamp, strDataAddr},
			[]interface{}{header.Timestamp(), signer}, "")
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		// return nil, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
		return nil, newConsensusError(
			trProposer,
			strErrTotalScore,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{parent.TotalScore() + score, header.TotalScore()}, "")
	}

	for _, u := range updates {
		if _, err := authority.Update(u.Address, u.Active); err != nil {
			return nil, err
		}
		if !candidates.Update(u.Address, u.Active) {
			// should never happen
			panic("something wrong with candidates list")
		}
	}

	return candidates, nil
}

func (c *Consensus) validateBlockBody(blk *block.Block) error {
	header := blk.Header()
	txs := blk.Transactions()
	if header.TxsRoot() != txs.RootHash() {
		// return consensusError(fmt.Sprintf("block txs root mismatch: want %v, have %v", header.TxsRoot(), txs.RootHash()))
		return newConsensusError(trBlockBody, strErrTxsRoot,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{header.TxsRoot(), txs.RootHash()}, "")
	}

	for _, tx := range txs {
		origin, err := tx.Origin()
		if err != nil {
			// return consensusError(fmt.Sprintf("tx signer unavailable: %v", err))
			return newConsensusError(trBlockBody, strErrSignature, nil, nil, err.Error())
		}

		if header.Number() >= c.forkConfig.BLOCKLIST && thor.IsOriginBlocked(origin) {
			// return consensusError(fmt.Sprintf("tx origin blocked got packed: %v", origin))
			return newConsensusError(trBlockBody, strErrBlockedTxOrign,
				[]string{strDataAddr}, []interface{}{origin}, "")
		}

		switch {
		case tx.ChainTag() != c.chain.Tag():
			// return consensusError(fmt.Sprintf("tx chain tag mismatch: want %v, have %v", c.chain.Tag(), tx.ChainTag()))
			return newConsensusError(trBlockBody, strErrChainTag,
				[]string{strDataExpected, strDataCurr},
				[]interface{}{c.chain.Tag(), tx.ChainTag()}, "")
		case header.Number() < tx.BlockRef().Number():
			// return consensusError(fmt.Sprintf("tx ref future block: ref %v, current %v", tx.BlockRef().Number(), header.Number()))
			return newConsensusError(trBlockBody, strErrFutureTx,
				[]string{strDataRef, strDataCurr},
				[]interface{}{tx.BlockRef().Number(), header.Number()}, "")
		case tx.IsExpired(header.Number()):
			// return consensusError(fmt.Sprintf("tx expired: ref %v, current %v, expiration %v", tx.BlockRef().Number(), header.Number(), tx.Expiration()))
			return newConsensusError(trBlockBody, strErrExpiredTx,
				[]string{strDataRef, strDataCurr, strDataExp},
				[]interface{}{tx.BlockRef().Number(), header.Number(), tx.Expiration()}, "")
		}

		if err := tx.TestFeatures(header.TxsFeatures()); err != nil {
			// return consensusError("invalid tx: " + err.Error())
			return newConsensusError(trBlockBody, "test tx features", nil, nil, err.Error())
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
			return nil, nil, newConsensusError("verifyBlock: ", "tx already exists", nil, nil, "")
		}

		// check depended tx
		if dep := tx.DependsOn(); dep != nil {
			found, reverted, err := findTx(*dep)
			if err != nil {
				return nil, nil, err
			}
			if !found {
				return nil, nil, newConsensusError("verifyBlock: ", "tx dep broken", nil, nil, "")
			}

			if reverted {
				return nil, nil, newConsensusError("verifyBlock: ", "tx dep reverted", nil, nil, "")
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
		// return nil, nil, consensusError(fmt.Sprintf("block gas used mismatch: want %v, have %v", header.GasUsed(), totalGasUsed))
		return nil, nil, newConsensusError("verifyBlock: ", strErrGasUsed,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{header.GasUsed(), totalGasUsed}, "")
	}

	receiptsRoot := receipts.RootHash()
	if header.ReceiptsRoot() != receiptsRoot {
		if c.correctReceiptsRoots[header.ID().String()] != receiptsRoot.String() {
			// return nil, nil, consensusError(fmt.Sprintf("block receipts root mismatch: want %v, have %v", header.ReceiptsRoot(), receiptsRoot))
			return nil, nil, newConsensusError("verifyBlock: ", strErrReceiptsRoot,
				[]string{strDataExpected, strDataCurr},
				[]interface{}{header.ReceiptsRoot(), receiptsRoot}, "")
		}
	}

	stage, err := state.Stage()
	if err != nil {
		return nil, nil, err
	}
	stateRoot := stage.Hash()

	if blk.Header().StateRoot() != stateRoot {
		// return nil, nil, consensusError(fmt.Sprintf("block state root mismatch: want %v, have %v", header.StateRoot(), stateRoot))
		return nil, nil, newConsensusError("verifyBlock: ", strErrStateRoot,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{header.StateRoot(), stateRoot}, "")
	}

	return stage, receipts, nil
}
