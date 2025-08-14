// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package reverts

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
)

type ErrRequire struct {
	message string
}

func NewRequireError(message string) *ErrRequire {
	return &ErrRequire{
		message: message,
	}
}

func (e *ErrRequire) Error() string {
	return e.message
}

func (e *ErrRequire) Bytes() []byte {
	if e == nil {
		return nil
	}

	// 4-byte selector for Error(string)
	selector, _ := hex.DecodeString("08c379a0")
	msgBytes := []byte(e.message)
	msgLen := uint64(len(msgBytes))

	// ABI-encode
	// selector + offset (32 bytes) + length (32 bytes) + data (padded to 32)
	encoded := make([]byte, 0, 4+32+32+((len(msgBytes)+31)/32)*32)
	encoded = append(encoded, selector...)

	// Offset is always 0x20 (32) after the selector
	offset := make([]byte, 32)
	binary.BigEndian.PutUint64(offset[24:], 32)
	encoded = append(encoded, offset...)

	// Length
	length := make([]byte, 32)
	binary.BigEndian.PutUint64(length[24:], msgLen)
	encoded = append(encoded, length...)

	// Message data padded
	data := make([]byte, ((len(msgBytes)+31)/32)*32)
	copy(data, msgBytes)
	encoded = append(encoded, data...)

	return encoded
}

func IsRevertErr(err any) bool {
	if err == nil {
		return false
	}
	e, ok := err.(error)
	if !ok {
		return false
	}
	var ve *ErrRequire
	if errors.As(e, &ve) {
		return ve != nil
	}
	return false
}
