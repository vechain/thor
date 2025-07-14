// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

func (c *Consensus) validate(
	state *state.State,
	block *block.Block,
	parent *block.Header,
	nowTimestamp uint64,
	blockConflicts uint32,
) (*state.Stage, tx.Receipts, error) {
	header := block.Header()

	staker := builtin.Staker.Native(state)
	posActive, activated, activeGroup, err := c.syncPOS(staker, header.Number())
	if err != nil {
		return nil, nil, err
	}
	if len(activeGroup) > 0 {
		// invalidate cache
		c.validatorsCache.Add(parent.ID(), activeGroup)
	}

	if err := c.validateBlockHeader(header, parent, nowTimestamp); err != nil {
		return nil, nil, err
	}

	var candidates *poa.Candidates
	if posActive {
		err = c.validateStakingProposer(header, parent, staker, activeGroup)
	} else {
		candidates, err = c.validateAuthorityProposer(header, parent, state)
	}
	if err != nil {
		return nil, nil, err
	}

	if activated {
		err := builtin.Energy.Native(state, parent.Timestamp()).StopEnergyGrowth()
		if err != nil {
			return nil, nil, err
		}
	}

	if err := c.validateBlockBody(block); err != nil {
		return nil, nil, err
	}

	stage, receipts, err := c.verifyBlock(block, state, blockConflicts, posActive)
	if err != nil {
		return nil, nil, err
	}

	if !posActive {
		if err := c.authorityCacheHandler(candidates, header, receipts); err != nil {
			return nil, nil, err
		}
	}

	return stage, receipts, nil
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

	if header.GasUsed() > header.GasLimit() {
		return consensusError(fmt.Sprintf("block gas used exceeds limit: limit %v, used %v", header.GasLimit(), header.GasUsed()))
	}

	if header.TotalScore() <= parent.TotalScore() {
		return consensusError(fmt.Sprintf("block total score invalid: parent %v, current %v", parent.TotalScore(), header.TotalScore()))
	}

	if !block.GasLimit(header.GasLimit()).IsValid(parent.GasLimit()) {
		return consensusError(fmt.Sprintf("block gas limit invalid: parent %v, current %v", parent.GasLimit(), header.GasLimit()))
	}

	signature := header.Signature()

	if header.Number() < c.forkConfig.VIP214 {
		if len(header.Alpha()) > 0 {
			return consensusError("invalid block, alpha should be empty before VIP214")
		}
		if len(signature) != 65 {
			return consensusError(fmt.Sprintf("block signature length invalid: want 65 have %v", len(signature)))
		}
	} else {
		if len(signature) != block.ComplexSigSize {
			return consensusError(fmt.Sprintf("block signature length invalid: want %d have %v", block.ComplexSigSize, len(signature)))
		}

		parentBeta, err := parent.Beta()
		if err != nil {
			return consensusError(fmt.Sprintf("failed to verify parent block's VRF Signature: %v", err))
		}

		var alpha []byte
		// initial value of chained VRF
		if len(parentBeta) == 0 {
			alpha = parent.StateRoot().Bytes()
		} else {
			alpha = parentBeta
		}
		if !bytes.Equal(header.Alpha(), alpha) {
			return consensusError(fmt.Sprintf("block alpha invalid: want %v, have %v", hexutil.Encode(alpha), hexutil.Encode(header.Alpha())))
		}

		if _, err := header.Beta(); err != nil {
			return consensusError(fmt.Sprintf("block VRF signature invalid: %v", err))
		}
	}

	if header.Number() < c.forkConfig.FINALITY {
		if header.COM() {
			return consensusError("invalid block: COM should not set before fork FINALITY")
		}
	}

	if header.Number() < c.forkConfig.GALACTICA {
		if header.BaseFee() != nil {
			return consensusError("invalid block: baseFee should not set before fork GALACTICA")
		}
	} else {
		if header.BaseFee() == nil {
			return consensusError("invalid block: baseFee is missing")
		}

		// Verify the baseFee is correct based on the parent header.
		expectedBaseFee := galactica.CalcBaseFee(parent, c.forkConfig)
		if header.BaseFee().Cmp(expectedBaseFee) != 0 {
			return consensusError(fmt.Sprintf("block baseFee invalid: have %s, want %s, parentBaseFee %s, parentGasUsed %d",
				header.BaseFee(), expectedBaseFee, parent.BaseFee(), parent.GasUsed()))
		}
	}

	return nil
}

func (c *Consensus) validateBlockBody(blk *block.Block) error {
	header := blk.Header()
	txs := blk.Transactions()
	if header.TxsRoot() != txs.RootHash() {
		return consensusError(fmt.Sprintf("block txs root mismatch: want %v, have %v", header.TxsRoot(), txs.RootHash()))
	}

	for _, tr := range txs {
		origin, err := tr.Origin()
		if err != nil {
			return consensusError(fmt.Sprintf("tx signer unavailable: %v", err))
		}

		if header.Number() >= c.forkConfig.BLOCKLIST && thor.IsOriginBlocked(origin) {
			return consensusError(fmt.Sprintf("tx origin blocked got packed: %v", origin))
		}

		delegator, err := tr.Delegator()
		if err != nil {
			return consensusError(fmt.Sprintf("tx delegator unavailable: %v", err))
		}
		if header.Number() >= c.forkConfig.BLOCKLIST && delegator != nil && thor.IsOriginBlocked(*delegator) {
			return consensusError(fmt.Sprintf("tx delegator blocked got packed: %v", delegator))
		}

		switch {
		case tr.ChainTag() != c.repo.ChainTag():
			return consensusError(fmt.Sprintf("tx chain tag mismatch: want %v, have %v", c.repo.ChainTag(), tr.ChainTag()))
		case header.Number() < tr.BlockRef().Number():
			return consensusError(fmt.Sprintf("tx ref future block: ref %v, current %v", tr.BlockRef().Number(), header.Number()))
		case tr.IsExpired(header.Number()):
			return consensusError(fmt.Sprintf("tx expired: ref %v, current %v, expiration %v", tr.BlockRef().Number(), header.Number(), tr.Expiration()))
		case header.Number() < c.forkConfig.GALACTICA && tr.Type() != tx.TypeLegacy:
			return consensusError("invalid tx: " + tx.ErrTxTypeNotSupported.Error())
		}

		if err := tr.TestFeatures(header.TxsFeatures()); err != nil {
			return consensusError("invalid tx: " + err.Error())
		}
	}

	return nil
}

func (c *Consensus) verifyBlock(blk *block.Block, state *state.State, blockConflicts uint32, posActive bool) (*state.Stage, tx.Receipts, error) {
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
			BaseFee:     header.BaseFee(),
		},
		c.forkConfig)

	findDep := func(txID thor.Bytes32) (found bool, reverted bool, err error) {
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

	hasTx := func(txid thor.Bytes32, txBlockRef uint32) (bool, error) {
		if _, ok := processedTxs[txid]; ok {
			return true, nil
		}
		return chain.HasTransaction(txid, txBlockRef)
	}

	for _, tx := range txs {
		// check if tx existed
		if found, err := hasTx(tx.ID(), tx.BlockRef().Number()); err != nil {
			return nil, nil, err
		} else if found {
			return nil, nil, consensusError("tx already exists")
		}

		// check depended tx
		if dep := tx.DependsOn(); dep != nil {
			found, reverted, err := findDep(*dep)
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

	if posActive {
		// TODO: We can reward priority fees here too
		staker := builtin.Staker.Native(state)
		energy := builtin.Energy.Native(state, header.Timestamp())
		if err := energy.DistributeRewards(blk.Header().Beneficiary(), signer, staker); err != nil {
			return nil, nil, err
		}
	}

	stage, err := state.Stage(trie.Version{Major: header.Number(), Minor: blockConflicts})
	if err != nil {
		return nil, nil, err
	}
	stateRoot := stage.Hash()

	if blk.Header().StateRoot() != stateRoot {
		return nil, nil, consensusError(fmt.Sprintf("block state root mismatch: want %v, have %v", header.StateRoot(), stateRoot))
	}

	return stage, receipts, nil
}

func (c *Consensus) syncPOS(staker *staker.Staker, current uint32) (active bool, activated bool, activeGroup map[thor.Bytes32]*staker.Validation, err error) {
	// still on PoA
	if c.forkConfig.HAYABUSA+c.forkConfig.HAYABUSA_TP > current {
		return false, false, nil, nil
	}
	// check if the staker contract is active
	active, err = staker.IsActive()
	if err != nil {
		return false, false, nil, err
	}

	// attempt to transition if we're on a transition block and the staker contract is not active
	if !active && current%c.forkConfig.HAYABUSA_TP == 0 {
		activated, err = staker.Transition(current)
		if err != nil {
			return false, false, nil, err
		}
		if activated {
			fmt.Println(HayabusaASCIIArt)
			log.Info("dPoS activated", "pkg", "consensus", "block", current)
			return true, true, nil, nil
		}
	}

	// perform housekeeping if the staker contract is active
	if active {
		_, activeGroup, err := staker.Housekeep(current)
		if err != nil {
			return false, false, nil, err
		}
		return true, false, activeGroup, nil
	}

	return active, false, nil, nil
}
