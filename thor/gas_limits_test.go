// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Each decode bound must admit every valid tx or block. When one fails after a
// gas limit change, raise the bound, not the assertion.
func TestDecodeBoundsAdmitValidInput(t *testing.T) {
	assert.GreaterOrEqual(t, MaxClausesPerTx*ClauseGas, MaxTxGasLimit,
		"a valid tx can carry more clauses than MaxClausesPerTx")
	assert.GreaterOrEqual(t, MaxTxsPerBlock*(TxGas+ClauseGas), GasLimitSoftLimit,
		"a valid block can carry more txs than MaxTxsPerBlock")
	assert.GreaterOrEqual(t, MaxClausesPerBlock*ClauseGas, GasLimitSoftLimit,
		"a valid block can carry more clauses than MaxClausesPerBlock")
}
