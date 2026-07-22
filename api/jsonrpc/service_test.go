// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummy struct{}

func (dummy) Echo(s string) (string, error)                   { return s, nil }
func (dummy) WithCtx(ctx context.Context, n int) (int, error) { return n, nil }
func (dummy) NoReturn()                                       {}
func (dummy) Boom() (string, error)                           { panic("boom") }
func (dummy) Bad() (int, int, int)                            { return 0, 0, 0 } // >2 returns, skipped
func (dummy) internal()                                       {}                 // unexported, skipped

func TestRegisterNameValidation(t *testing.T) {
	var r serviceRegistry
	assert.Error(t, r.registerName("", dummy{}))         // empty namespace
	assert.Error(t, r.registerName("empty", struct{}{})) // no suitable methods
	require.NoError(t, r.registerName("dummy", dummy{}))
}

func TestCallbackDiscoveryAndSignatures(t *testing.T) {
	var r serviceRegistry
	require.NoError(t, r.registerName("dummy", dummy{}))

	assert.NotNil(t, r.callback("dummy_echo"))
	assert.NotNil(t, r.callback("dummy_noReturn"))
	assert.Nil(t, r.callback("dummy_bad"))      // >2 returns skipped
	assert.Nil(t, r.callback("dummy_internal")) // unexported skipped
	assert.Nil(t, r.callback("noUnderscore"))

	echo := r.callback("dummy_echo")
	assert.False(t, echo.hasCtx)
	assert.Len(t, echo.argTypes, 1)
	assert.Equal(t, 1, echo.errPos)

	withCtx := r.callback("dummy_withCtx")
	assert.True(t, withCtx.hasCtx)
	assert.Len(t, withCtx.argTypes, 1) // ctx not counted
}

func TestParseArgs(t *testing.T) {
	var r serviceRegistry
	require.NoError(t, r.registerName("dummy", dummy{}))
	echo := r.callback("dummy_echo")

	// valid single positional arg
	args, err := echo.parseArgs(json.RawMessage(`["hi"]`))
	require.NoError(t, err)
	require.Len(t, args, 1)
	assert.Equal(t, "hi", args[0].String())

	// too many args -> -32602
	_, err = echo.parseArgs(json.RawMessage(`["a","b"]`))
	require.Error(t, err)
	assert.Equal(t, errcodeInvalidParams, err.(*jsonError).Code)

	// wrong type -> -32602
	_, err = echo.parseArgs(json.RawMessage(`[5]`))
	require.Error(t, err)
	assert.Equal(t, errcodeInvalidParams, err.(*jsonError).Code)

	// missing optional trailing arg -> zero value
	args, err = echo.parseArgs(json.RawMessage(`[]`))
	require.NoError(t, err)
	require.Len(t, args, 1)
	assert.Equal(t, "", args[0].String())
}

func TestCallResultErrorPanic(t *testing.T) {
	var r serviceRegistry
	require.NoError(t, r.registerName("dummy", dummy{}))
	ctx := context.Background()

	echo := r.callback("dummy_echo")
	args, _ := echo.parseArgs(json.RawMessage(`["yo"]`))
	res, err := echo.call(ctx, args)
	require.NoError(t, err)
	assert.Equal(t, "yo", res)

	no := r.callback("dummy_noReturn")
	res, err = no.call(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, res)

	boom := r.callback("dummy_boom")
	_, err = boom.call(ctx, nil)
	require.Error(t, err)
	assert.Equal(t, errcodeInternal, err.(*jsonError).Code)
}
