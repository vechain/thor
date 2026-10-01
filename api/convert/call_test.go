// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// httpStatus returns the status and body a handler responds with for err.
func httpStatus(t *testing.T, err error) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	restutil.WrapHandlerFunc(func(http.ResponseWriter, *http.Request) error { return err })(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestCallGas(t *testing.T) {
	gas, err := CallGas(0, 1000)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), gas, "zero gas defaults to the limit")

	gas, err = CallGas(1000, 1000)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), gas)

	gas, err = CallGas(21000, 50000)
	require.NoError(t, err)
	assert.Equal(t, uint64(21000), gas)

	_, err = CallGas(1001, 1000)
	status, body := httpStatus(t, err)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "gas: exceeds limit", body)
}

func TestBuildCallTxContext(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		ctx, err := BuildCallTxContext(2, nil, nil, nil, nil, 0, "")
		require.NoError(t, err)
		assert.Equal(t, uint32(2), ctx.ClauseCount)
		assert.Equal(t, uint32(0), ctx.Expiration)
		assert.Equal(t, 0, ctx.GasPrice.Sign())
		assert.Equal(t, 0, ctx.ProvedWork.Sign())
		assert.Equal(t, thor.Address{}, ctx.Origin)
		assert.Equal(t, thor.Address{}, ctx.GasPayer)
		assert.Equal(t, tx.BlockRef{}, ctx.BlockRef)
	})

	t.Run("values", func(t *testing.T) {
		caller, payer := thor.BytesToAddress([]byte("caller")), thor.BytesToAddress([]byte("payer"))
		ctx, err := BuildCallTxContext(
			1,
			(*math.HexOrDecimal256)(big.NewInt(7)), (*math.HexOrDecimal256)(big.NewInt(9)),
			&caller, &payer,
			720,
			"0x0000000a00000000",
		)
		require.NoError(t, err)
		assert.Equal(t, uint32(1), ctx.ClauseCount)
		assert.Equal(t, uint32(720), ctx.Expiration)
		assert.Equal(t, big.NewInt(7), ctx.GasPrice)
		assert.Equal(t, big.NewInt(9), ctx.ProvedWork)
		assert.Equal(t, caller, ctx.Origin)
		assert.Equal(t, payer, ctx.GasPayer)
		assert.Equal(t, tx.NewBlockRef(10), ctx.BlockRef)
	})

	for name, tc := range map[string]struct{ blockRef, body string }{
		"bad hex":        {"0xzz", "blockRef: invalid hex string"},
		"invalid length": {"0x01", "blockRef: invalid length"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := BuildCallTxContext(1, nil, nil, nil, nil, 0, tc.blockRef)
			status, body := httpStatus(t, err)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, tc.body, body)
		})
	}
}

func TestBuildBlockContext(t *testing.T) {
	key, err := crypto.HexToECDSA(strings.Repeat("33", 32))
	require.NoError(t, err)
	blk := new(block.Builder).
		ParentID(thor.Bytes32{0, 0, 0, 9}).
		Timestamp(1_700_000_000).
		TotalScore(100).
		GasLimit(40_000_000).
		Beneficiary(thor.BytesToAddress([]byte("beneficiary"))).
		BaseFee(big.NewInt(10_000_000_000_000)).
		Build()
	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), key)
	require.NoError(t, err)
	header := blk.WithSignature(sig).Header()
	signer, err := header.Signer()
	require.NoError(t, err)
	require.NotEqual(t, thor.Address{}, signer)

	ctx := BuildBlockContext(header)
	assert.Equal(t, header.Beneficiary(), ctx.Beneficiary)
	assert.Equal(t, signer, ctx.Signer)
	assert.Equal(t, header.Number(), ctx.Number)
	assert.Equal(t, header.Timestamp(), ctx.Time)
	assert.Equal(t, header.GasLimit(), ctx.GasLimit)
	assert.Equal(t, header.TotalScore(), ctx.TotalScore)
	assert.Equal(t, header.BaseFee(), ctx.BaseFee)
}
