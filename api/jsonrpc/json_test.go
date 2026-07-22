// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type dataErr struct{}

func (dataErr) Error() string           { return "boom" }
func (dataErr) ErrorCode() int          { return -32050 }
func (dataErr) ErrorData() interface{}  { return map[string]int{"x": 1} }

func TestToJSONError(t *testing.T) {
	// plain jsonError passes through unchanged
	je := &jsonError{Code: errcodeInvalidParams, Message: "bad"}
	assert.Same(t, je, toJSONError(je))

	// generic error maps to -32000
	got := toJSONError(errors.New("plain"))
	assert.Equal(t, errcodeDefault, got.Code)
	assert.Equal(t, "plain", got.Message)
	assert.Nil(t, got.Data)

	// DataError carries code + data
	got = toJSONError(dataErr{})
	assert.Equal(t, -32050, got.Code)
	assert.Equal(t, map[string]int{"x": 1}, got.Data)
}
