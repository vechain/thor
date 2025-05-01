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
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

var (
	logger = log.WithContext("pkg", "consensus")
)

type cacheHandler func(receipts tx.Receipts) error

func (c *Consensus) validate(
	state *state.State,
	block *block.Block,
	parent *block.Header,
	nowTimestamp uint64,
	blockConflicts uint32,
) (*state.Stage, tx.Receipts, error) {
	header := block.Header()

	if err := c.validateBlockHeader(header, parent, nowTimestamp); err != nil {
		return nil, nil, err
	}

	cacheHandler, posActive, err := c.validateProposer(header, parent, state)
	if err != nil {
		return nil, nil, err
	}

	if err := c.validateBlockBody(block); err != nil {
		return nil, nil, err
	}

	stage, receipts, err := c.verifyBlock(block, state, blockConflicts, posActive)
	if err != nil {
		return nil, nil, err
	}

	if err = cacheHandler(receipts); err != nil {
		return nil, nil, err
	}

	return stage, receipts, nil
}

func (c *Consensus) validateProposer(header *block.Header, parent *block.Header, state *state.State) (cacheHandler, bool, error) {
	posActive, err := builtin.Staker.Native(state).IsActive()
	if err != nil {
		return nil, false, err
	}
	logger.Debug("validating block proposer", "pos", posActive)
	var handler cacheHandler
	if posActive {
		handler, err = c.validateStakingProposer(header, parent, state)
	} else {
		handler, err = c.validateAuthorityProposer(header, parent, state)
	}
	return handler, posActive, err
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

	return nil
}

func (c *Consensus) validateBlockBody(blk *block.Block) error {
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
