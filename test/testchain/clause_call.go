// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"math"
	"math/big"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// ClauseResult is the outcome of executing a single clause.
type ClauseResult struct {
	Data []byte
	// GasUsed is clause execution gas only; it excludes the intrinsic
	// transaction gas. It stays valid when VMErr is non-nil, which is what
	// makes the cost of a reverting call observable.
	GasUsed uint64
	// VMErr is non-nil when the clause reverted or hit a VM error.
	VMErr error
}

// ExecClause executes the clause referenced by clauseIdx against the best
// block's state, with the rest of tx passed as is. A VM revert is reported in
// the result rather than as an error, so callers can inspect the gas it burned.
func (c *Chain) ExecClause(account genesis.DevAccount, trx *tx.Transaction, clauseIdx int) (*ClauseResult, error) {
	ch := c.repo.NewBestChain()
	summary, err := c.repo.GetBlockSummary(ch.HeadID())
	if err != nil {
		return nil, err
	}
	st := state.New(c.db, trie.Root{Hash: summary.Header.StateRoot(), Ver: trie.Version{Major: summary.Header.Number()}})
	rt := runtime.New(
		ch,
		st,
		&xenv.BlockContext{Number: summary.Header.Number(), Time: summary.Header.Timestamp(), TotalScore: summary.Header.TotalScore(), Signer: account.Address},
		c.forkConfig,
	)
	maxGas := uint64(math.MaxUint32)
	exec, _ := rt.PrepareClause(trx.Clauses()[clauseIdx],
		0, maxGas, &xenv.TransactionContext{
			ID:         trx.ID(),
			Origin:     account.Address,
			GasPrice:   &big.Int{},
			GasPayer:   account.Address,
			ProvedWork: trx.UnprovedWork(),
			BlockRef:   trx.BlockRef(),
			Expiration: trx.Expiration(),
		})

	out, _, err := exec()
	if err != nil {
		return nil, err
	}
	return &ClauseResult{
		Data:    out.Data,
		GasUsed: maxGas - out.LeftOverGas,
		VMErr:   out.VMErr,
	}, nil
}

// ClauseCall executes contract call with clause referenced by the clauseIdx parameter, the rest of tx is passed as is.
// A VM revert is folded into the returned error and the gas it burned is discarded;
// use ExecClause when that gas matters.
func (c *Chain) ClauseCall(account genesis.DevAccount, trx *tx.Transaction, clauseIdx int) ([]byte, uint64, error) {
	res, err := c.ExecClause(account, trx, clauseIdx)
	if err != nil {
		return nil, 0, err
	}
	if res.VMErr != nil {
		return nil, 0, res.VMErr
	}
	return res.Data, res.GasUsed, nil
}
