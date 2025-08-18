// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package xenv

import (
	"errors"

	"github.com/vechain/thor/v2/abi"
)

var revertABI = []byte(`[{"name": "Error","type": "function","inputs": [{"name": "message","type": "string"}]}]`)

type errReverted struct {
	message string
}

func (e *errReverted) Error() string {
	return "builtin reverted: " + e.message
}

func (e *errReverted) Bytes() []byte {
	abi, _ := abi.New(revertABI)
	method, _ := abi.MethodByName("Error")
	data, _ := method.EncodeInput(e.message)
	return data
}

func isReverted(err any) bool {
	e, ok := err.(error)
	if !ok {
		return false
	}
	var target *errReverted
	return errors.As(e, &target)
}
