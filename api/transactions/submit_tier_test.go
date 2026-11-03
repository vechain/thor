// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// tierRecorderPool records which pool method the API used to submit. The
// submission payloads are not validated by it, so the tx contents only need
// to be decodable.
type tierRecorderPool struct {
	txpool.Pool // unimplemented base; only the submit path is exercised

	adds     int
	addLocal int
}

func (p *tierRecorderPool) Add(*tx.Transaction) error      { p.adds++; return nil }
func (p *tierRecorderPool) AddLocal(*tx.Transaction) error { p.addLocal++; return nil }

// TestSubmitTierWiring pins the flag routing: by default API submissions
// enter the pool as remote (pool.Add); with local privileges enabled they
// enter the local tier (pool.AddLocal).
func TestSubmitTierWiring(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.NoError(t, err)
	clause := tx.NewClause(nil)
	txb := new(tx.Builder).
		ChainTag(0).
		Expiration(100).
		Gas(21000).
		GasPriceCoef(0).
		BlockRef(tx.NewBlockRef(0)).
		Clause(clause)
	signed := tx.MustSign(txb.Build(), key)
	raw, err := signed.MarshalBinary()
	assert.NoError(t, err)
	payload, err := json.Marshal(map[string]string{"raw": hexutil.Encode(raw)})
	assert.NoError(t, err)

	for _, c := range []struct {
		name        string
		privileges  bool
		expectAdd   int
		expectLocal int
	}{
		{"default: remote tier", false, 1, 0},
		{"privileges: local tier", true, 0, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			pool := &tierRecorderPool{}
			router := mux.NewRouter()
			New(nil, pool, c.privileges).Mount(router, "/transactions")
			ts := httptest.NewServer(router)
			defer ts.Close()

			resp, err := http.Post(ts.URL+"/transactions", "application/json", bytes.NewReader(payload))
			assert.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			assert.Equal(t, c.expectAdd, pool.adds)
			assert.Equal(t, c.expectLocal, pool.addLocal)
		})
	}
}
