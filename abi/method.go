// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package abi

import (
	"bytes"
	"errors"

	ethabi "github.com/vechain/thor/v2/abi/ethabi"
)

// MethodID method id.
type MethodID [4]byte

// EmptyMethodID represents an empty method ID (used for constructors).
var EmptyMethodID = MethodID{}

// IsEmpty returns true if the MethodID is empty.
func (id MethodID) IsEmpty() bool {
	return id == EmptyMethodID
}

// Method see abi.Method in go-ethereum.
type Method struct {
	id     MethodID
	method *ethabi.Method
}

// ID returns method id.
func (m *Method) ID() MethodID {
	return m.id
}

// Name returns method name.
func (m *Method) Name() string {
	return m.method.Name
}

// Const returns if the method is const.
func (m *Method) Const() bool {
	return m.method.Constant || m.method.StateMutability == "view" || m.method.StateMutability == "pure"
}

// EncodeInput encode args to data.
func (m *Method) EncodeInput(args ...any) ([]byte, error) {
	data, err := m.method.Inputs.Pack(args...)
	if err != nil {
		return nil, err
	}

	if m.id.IsEmpty() {
		return data, nil
	}

	return append(m.id[:], data...), nil
}

// DecodeInput decode input data into args.
func (m *Method) DecodeInput(input []byte, v any) error {
	if m.id.IsEmpty() {
		if len(input) != 0 {
			return UnpackIntoInterface(&m.method.Inputs, input, v)
		}
		// if constructor with no parameters
		return nil
	}

	if !bytes.HasPrefix(input, m.id[:]) {
		return errors.New("input has incorrect prefix")
	}

	return UnpackIntoInterface(&m.method.Inputs, input[len(m.id):], v)
}

// EncodeOutput encode output args to data.
func (m *Method) EncodeOutput(args ...any) ([]byte, error) {
	return m.method.Outputs.Pack(args...)
}

// DecodeOutput decode output data.
func (m *Method) DecodeOutput(output []byte, v any) error {
	if len(output)%32 != 0 {
		return errors.New("output has incorrect length")
	}
	return UnpackIntoInterface(&m.method.Outputs, output, v)
}

// ExtractMethodID extract method id from input data.
func ExtractMethodID(input []byte) (id MethodID, err error) {
	if len(input) < len(id) {
		err = errors.New("input data too short")
		return
	}
	copy(id[:], input)
	return
}
