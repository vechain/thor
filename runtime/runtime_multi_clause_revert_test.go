// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// TestMultiClauseRevertPropagation guards the multi-clause all-or-nothing
// semantic: when any clause hits a VMErr, the whole tx must be marked reverted
// and prior clauses' state changes rolled back. Regression for a bug where the
// non-eth path returned without setting reverted=true / txOutputs=nil.
func TestMultiClauseRevertPropagation(t *testing.T) {
	origin := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	db := muxdb.NewMem()
	g, fc := genesis.NewDevnet()
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	prevBal, err := st.GetBalance(recipient.Address)
	assert.Nil(t, err)

	transferValue := big.NewInt(1000)
	to := recipient.Address

	// Clause 0: VET transfer (succeeds). Clause 1: deploy contract whose
	// init code is a single INVALID opcode (0xfe) → ErrInvalidOpCode → VMErr.
	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(repo.ChainTag()).
		Gas(200_000).
		GasPriceCoef(0).
		Expiration(32).
		Clause(tx.NewClause(&to).WithValue(transferValue)).
		Clause(tx.NewClause(nil).WithData([]byte{0xfe})).
		Nonce(1).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{
		GasLimit: b0.Header().GasLimit(),
		BaseFee:  big.NewInt(thor.InitialBaseFee),
	}, fc)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err, "ExecuteTransaction should not error on clause-level revert")
	assert.NotNil(t, receipt)
	assert.True(t, receipt.Reverted, "multi-clause tx must be marked reverted when any clause fails")
	assert.Nil(t, receipt.Outputs, "outputs must be cleared on revert")

	// Recipient balance must be unchanged: clause 0's transfer was reverted.
	currBal, err := st.GetBalance(recipient.Address)
	assert.Nil(t, err)
	assert.Equal(t, prevBal, currBal, "prior clause's state changes must be rolled back")
}
