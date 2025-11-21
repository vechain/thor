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

// ClauseCall executes contract call with clause referenced by the clauseIdx parameter, the rest of tx is passed as is.
func (c *Chain) ClauseCall(account genesis.DevAccount, trx *tx.Transaction, clauseIdx int) ([]byte, uint64, error) {
	ch := c.repo.NewBestChain()
	summary, err := c.repo.GetBlockSummary(ch.HeadID())
	if err != nil {
		return nil, 0, err
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
		return nil, 0, err
	}
	if out.VMErr != nil {
		return nil, 0, out.VMErr
	}
	return out.Data, maxGas - out.LeftOverGas, err
}
