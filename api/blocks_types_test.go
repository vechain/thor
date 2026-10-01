// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestBuildJSONOutputNilTopics(t *testing.T) {
	to := thor.BytesToAddress([]byte("to"))
	clause := tx.NewClause(&to)
	output := &tx.Output{
		Events: tx.Events{&tx.Event{Address: thor.BytesToAddress([]byte("addr")), Topics: nil, Data: nil}},
	}

	jo := buildJSONOutput(thor.Bytes32{}, 0, clause, output)

	assert.Len(t, jo.Events, 1)
	assert.NotNil(t, jo.Events[0].Topics)
	assert.Len(t, jo.Events[0].Topics, 0)

	data, err := json.Marshal(jo)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"topics":[]`)
}
