package abi

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/crypto"
)

// revertSelector is a special function selector for revert reason unpacking.
var revertSelector = crypto.Keccak256([]byte("Error(string)"))[:4]

// panicSelector is a special function selector for panic reason unpacking.
var panicSelector = crypto.Keccak256([]byte("Panic(uint256)"))[:4]

// panicReasons map is for readable panic codes
// see this linkage for the details
// https://docs.soliditylang.org/en/v0.8.21/control-structures.html#panic-via-assert-and-error-via-require
// the reason string list is copied from ether.js
// https://github.com/ethers-io/ethers.js/blob/fa3a883ff7c88611ce766f58bdd4b8ac90814470/src.ts/abi/interface.ts#L207-L218
var panicReasons = map[uint64]string{
	0x00: "generic panic",
	0x01: "assert(false)",
	0x11: "arithmetic underflow or overflow",
	0x12: "division or modulo by zero",
	0x21: "enum overflow",
	0x22: "invalid encoded storage byte array accessed",
	0x31: "out-of-bounds array access; popping on an empty array",
	0x32: "out-of-bounds access of an array or bytesN",
	0x41: "out of memory",
	0x51: "uninitialized function",
}

// UnpackRevert resolves the abi-encoded revert reason. According to the solidity
// spec https://solidity.readthedocs.io/en/latest/control-structures.html#revert,
// the provided revert reason is abi-encoded as if it were a call to function
// `Error(string)` or `Panic(uint256)`. So it's a special tool for it.
func UnpackRevert(data []byte) (string, error) {
	if len(data) < 4 {
		return "", errors.New("invalid data for unpacking")
	}
	switch {
	case bytes.Equal(data[:4], revertSelector):
		typ, err := ethabi.NewType("string")
		if err != nil {
			return "", err
		}
		var unpacked string
		if err := (ethabi.Arguments{{Type: typ}}).Unpack(&unpacked, data[4:]); err != nil {
			return "", err
		}
		return unpacked, nil
	case bytes.Equal(data[:4], panicSelector):
		typ, err := ethabi.NewType("uint256")
		if err != nil {
			return "", err
		}
		var pCode big.Int
		if err := (ethabi.Arguments{{Type: typ}}).Unpack(&pCode, data[4:]); err != nil {
			return "", err
		}
		// uint64 safety check for future
		// but the code is not bigger than MAX(uint64) now
		if pCode.IsUint64() {
			if reason, ok := panicReasons[pCode.Uint64()]; ok {
				return reason, nil
			}
		}
		return fmt.Sprintf("unknown panic code: %#x", pCode), nil
	default:
		return "", errors.New("invalid data for unpacking")
	}
}
