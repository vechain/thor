// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codeRecorderStateDB captures the exact slice handed to SetCode so the test can
// inspect its cap, i.e. how much backing array the deployed code keeps alive.
type codeRecorderStateDB struct {
	NoopStateDB
	code []byte
}

func (s *codeRecorderStateDB) SetCode(_ common.Address, code []byte) { s.code = code }

// aliasInitCode builds initcode that inflates frame memory to memBytes and then
// RETURNs only 3 bytes taken from offset 0.
func aliasInitCode(memBytes uint64, tag uint16) []byte {
	off := uint32(memBytes - 32)
	return []byte{
		0x60, 0x00, // PUSH1 0                 (value)
		0x63, byte(off >> 24), byte(off >> 16), byte(off >> 8), byte(off), // PUSH4 off
		0x52,       // MSTORE -> memory grows to memBytes
		0x60, 0x60, // PUSH1 0x60
		0x60, 0x00, // PUSH1 0
		0x53,                 // MSTORE8 mem[0] = 0x60 (not 0xEF, EIP-3541 safe)
		0x60, byte(tag >> 8), // PUSH1 hi
		0x60, 0x01, // PUSH1 1
		0x53,            // MSTORE8 mem[1]
		0x60, byte(tag), // PUSH1 lo
		0x60, 0x02, // PUSH1 2
		0x53,       // MSTORE8 mem[2]
		0x60, 0x03, // PUSH1 3                 (size)
		0x60, 0x00, // PUSH1 0                 (offset)
		0xf3, // RETURN
	}
}

func newAliasEVM(statedb StateDB) *EVM {
	return NewEVM(
		Context{
			BlockNumber:        big.NewInt(1),
			GasPrice:           big.NewInt(1),
			CanTransfer:        NoopCanTransfer,
			Transfer:           NoopTransfer,
			NewContractAddress: newContractAddress,
		},
		statedb,
		&ChainConfig{ChainConfig: *params.TestChainConfig},
		Config{},
	)
}

// TestDeployedCodeDoesNotRetainFrameMemory is the mechanical proof of the leak:
// a 3-byte contract must not keep a 128 KiB frame buffer alive.
func TestDeployedCodeDoesNotRetainFrameMemory(t *testing.T) {
	const memBytes = 128 * 1024

	sdb := &codeRecorderStateDB{}
	evm := newAliasEVM(sdb)

	_, _, _, err := evm.Create(AccountRef(common.HexToAddress("0x01")), aliasInitCode(memBytes, 0x0102), 10_000_000, big.NewInt(0))
	require.NoError(t, err)
	require.NotNil(t, sdb.code)

	assert.Equal(t, []byte{0x60, 0x01, 0x02}, sdb.code, "deployed code bytes")
	t.Logf("len(code)=%d cap(code)=%d frame memory=%d", len(sdb.code), cap(sdb.code), memBytes)
	assert.Less(t, cap(sdb.code), 64,
		"deployed code aliases the %d-byte frame memory (cap=%d)", memBytes, cap(sdb.code))
}

// TestReturnRevertBytesUnchanged pins the observable semantics of RETURN/REVERT
// across the GetPtr -> GetCopy change.
func TestReturnRevertBytesUnchanged(t *testing.T) {
	// program layout: MSTORE(0, 0x11..0x30) then <op>(offset, size)
	build := func(op byte, offset, size uint64) []byte {
		var val [32]byte
		for i := range val {
			val[i] = byte(0x11 + i)
		}
		code := []byte{0x7f} // PUSH32
		code = append(code, val[:]...)
		code = append(code,
			0x60, 0x00, // PUSH1 0
			0x52, // MSTORE
		)
		code = append(code, 0x63, byte(size>>24), byte(size>>16), byte(size>>8), byte(size)) // PUSH4 size
		code = append(code, 0x63, byte(offset>>24), byte(offset>>16), byte(offset>>8), byte(offset))
		return append(code, op)
	}

	for _, tt := range []struct {
		name     string
		op       byte
		offset   uint64
		size     uint64
		expected []byte
	}{
		{"return size=0", 0xf3, 0, 0, nil},
		{"return size=0 far offset", 0xf3, 1 << 20, 0, nil},
		{"return whole word", 0xf3, 0, 32, []byte{
			0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
			0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30,
		}},
		{"return spans past written word", 0xf3, 30, 4, []byte{0x2f, 0x30, 0x00, 0x00}},
		{"return beyond memory end", 0xf3, 64, 8, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{"revert size=0", 0xfd, 0, 0, nil},
		{"revert reason", 0xfd, 0, 8, []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}},
		{"revert beyond memory end", 0xfd, 96, 4, []byte{0, 0, 0, 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			evm := newAliasEVM(NoopStateDB{})
			contract := NewContract(AccountRef(common.HexToAddress("0x01")), AccountRef(common.HexToAddress("0x02")), big.NewInt(0), 10_000_000)
			code := build(tt.op, tt.offset, tt.size)
			contract.SetCallCode(&common.Address{}, common.Hash{}, code)

			ret, err := evm.interpreter.Run(contract, nil)
			if tt.op == 0xfd {
				assert.Equal(t, ErrExecutionReverted, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, ret)
		})
	}
}
