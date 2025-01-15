// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

// precompiledTest defines the input/output pairs for precompiled contract tests.
type precompiledTest struct {
	Input, Expected string
	Gas             uint64
	Name            string
	NoBenchmark     bool // Benchmark primarily the worst-cases
}

// precompiledFailureTest defines the input/error pairs for precompiled
// contract failure tests.
type precompiledFailureTest struct {
	Input         string
	ExpectedError string
	Name          string
}

// allPrecompiles does not map to the actual set of precompiles, as it also contains
// repriced versions of precompiles at certain slots
var allPrecompiles = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}):    &safeEcrecover{},
	common.BytesToAddress([]byte{2}):    &sha256hash{},
	common.BytesToAddress([]byte{3}):    &ripemd160hash{},
	common.BytesToAddress([]byte{4}):    &dataCopy{},
	common.BytesToAddress([]byte{5}):    &bigModExp{eip2565: false},
	common.BytesToAddress([]byte{0xf5}): &bigModExp{eip2565: true},
	common.BytesToAddress([]byte{6}):    &bn256Add{eip1108: false},
	common.BytesToAddress([]byte{0xf6}): &bn256Add{eip1108: true},
	common.BytesToAddress([]byte{7}):    &bn256ScalarMul{eip1108: false},
	common.BytesToAddress([]byte{0xf7}): &bn256ScalarMul{eip1108: true},
	common.BytesToAddress([]byte{8}):    &bn256Pairing{eip1108: false},
	common.BytesToAddress([]byte{0xf8}): &bn256Pairing{eip1108: true},
	common.BytesToAddress([]byte{9}):    &blake2F{},
}

func testPrecompiled(addr string, test precompiledTest, t *testing.T) {
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	gas := p.RequiredGas(in)
	contract := NewContract(AccountRef(common.HexToAddress("1337")),
		nil, new(big.Int), gas)

	t.Run(fmt.Sprintf("%s-Gas=%d", test.Name, gas), func(t *testing.T) {
		if res, err := RunPrecompiledContract(p, in, contract); err != nil {
			t.Error(err)
		} else if common.Bytes2Hex(res) != test.Expected {
			t.Errorf("Expected %v, got %v", test.Expected, common.Bytes2Hex(res))
		}
		if expGas := test.Gas; expGas != gas {
			t.Errorf("%v: gas wrong, expected %d, got %d", test.Name, expGas, gas)
		}
		// Verify that the precompile did not touch the input buffer
		exp := common.Hex2Bytes(test.Input)
		if !bytes.Equal(in, exp) {
			t.Errorf("Precompiled %v modified input data", addr)
		}
	})
}

func testPrecompiledOOG(addr string, test precompiledTest, t *testing.T) {
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	gas := p.RequiredGas(in) - 1
	contract := NewContract(AccountRef(common.HexToAddress("1337")),
		nil, new(big.Int), gas)
	t.Run(fmt.Sprintf("%s-Gas=%d", test.Name, gas), func(t *testing.T) {
		_, err := RunPrecompiledContract(p, in, contract)
		if err.Error() != "out of gas" {
			t.Errorf("Expected error [out of gas], got [%v]", err)
		}
		// Verify that the precompile did not touch the input buffer
		exp := common.Hex2Bytes(test.Input)
		if !bytes.Equal(in, exp) {
			t.Errorf("Precompiled %v modified input data", addr)
		}
	})
}

func testPrecompiledFailure(addr string, test precompiledFailureTest, t *testing.T) {
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	gas := p.RequiredGas(in)
	contract := NewContract(AccountRef(common.HexToAddress("1337")),
		nil, new(big.Int), gas)
	t.Run(test.Name, func(t *testing.T) {
		_, err := RunPrecompiledContract(p, in, contract)
		if err.Error() != test.ExpectedError {
			t.Errorf("Expected error [%v], got [%v]", test.ExpectedError, err)
		}
		// Verify that the precompile did not touch the input buffer
		exp := common.Hex2Bytes(test.Input)
		if !bytes.Equal(in, exp) {
			t.Errorf("Precompiled %v modified input data", addr)
		}
	})
}

func benchmarkPrecompiled(addr string, test precompiledTest, bench *testing.B) {
	if test.NoBenchmark {
		return
	}
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	reqGas := p.RequiredGas(in)

	var (
		res  []byte
		err  error
		data = make([]byte, len(in))
	)

	bench.Run(fmt.Sprintf("%s-Gas=%d", test.Name, reqGas), func(bench *testing.B) {
		bench.ReportAllocs()
		start := time.Now()
		bench.ResetTimer()
		for i := 0; i < bench.N; i++ {
			copy(data, in)
			contract := NewContract(AccountRef(common.HexToAddress("1337")),
				nil, new(big.Int), reqGas)
			res, err = RunPrecompiledContract(p, data, contract)
		}
		bench.StopTimer()
		elapsed := max(uint64(time.Since(start)), 1)
		gasUsed := reqGas * uint64(bench.N)
		bench.ReportMetric(float64(reqGas), "gas/op")
		// Keep it as uint64, multiply 100 to get two digit float later
		mgasps := (100 * 1000 * gasUsed) / elapsed
		bench.ReportMetric(float64(mgasps)/100, "mgas/s")
		//Check if it is correct
		if err != nil {
			bench.Error(err)
			return
		}
		if common.Bytes2Hex(res) != test.Expected {
			bench.Errorf("Expected %v, got %v", test.Expected, common.Bytes2Hex(res))
			return
		}
	})
}

// Benchmarks the sample inputs from the ECRECOVER precompile.
func BenchmarkPrecompiledEcrecover(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "000000000000000000000000ceaccac640adf55b2028469bd36ba501f28b699d",
		Name:     "",
	}
	benchmarkPrecompiled("01", t, bench)
}

// Benchmarks the sample inputs from the SHA256 precompile.
func BenchmarkPrecompiledSha256(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "811c7003375852fabd0d362e40e68607a12bdabae61a7d068fe5fdd1dbbf2a5d",
		Name:     "128",
	}
	benchmarkPrecompiled("02", t, bench)
}

// Benchmarks the sample inputs from the RIPEMD precompile.
func BenchmarkPrecompiledRipeMD(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "0000000000000000000000009215b8d9882ff46f0dfde6684d78e831467f65e6",
		Name:     "128",
	}
	benchmarkPrecompiled("03", t, bench)
}

// Benchmarks the sample inputs from the identiy precompile.
func BenchmarkPrecompiledIdentity(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Name:     "128",
	}
	benchmarkPrecompiled("04", t, bench)
}

// Tests the sample inputs from the ModExp EIP 198.
func TestPrecompiledModExp(t *testing.T)      { testJSON("modexp", "05", t) }
func BenchmarkPrecompiledModExp(b *testing.B) { benchJSON("modexp", "05", b) }

func TestPrecompiledModExpEip2565(t *testing.T)      { testJSON("modexp_eip2565", "f5", t) }
func BenchmarkPrecompiledModExpEip2565(b *testing.B) { benchJSON("modexp_eip2565", "f5", b) }

// Tests the sample inputs from the elliptic curve addition EIP 213.
func TestPrecompiledBn256Add(t *testing.T)      { testJSON("bn256Add", "06", t) }
func BenchmarkPrecompiledBn256Add(b *testing.B) { benchJSON("bn256Add", "06", b) }

func TestPrecompiledBn256AddEip1108(t *testing.T)      { testJSON("bn256Add_eip1108", "f6", t) }
func BenchmarkPrecompiledBn256AddEip1108(b *testing.B) { benchJSON("bn256Add_eip1108", "f6", b) }

// Tests OOG
func TestPrecompiledModExpOOG(t *testing.T) {
	modexpTests, err := loadJSON("modexp")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range modexpTests {
		testPrecompiledOOG("05", test, t)
	}
}

// Tests the sample inputs from the elliptic curve scalar multiplication EIP 213.
func TestPrecompiledBn256ScalarMul(t *testing.T)      { testJSON("bn256ScalarMul", "07", t) }
func BenchmarkPrecompiledBn256ScalarMul(b *testing.B) { benchJSON("bn256ScalarMul", "07", b) }

func TestPrecompiledBn256ScalarMulEip1108(t *testing.T) { testJSON("bn256ScalarMul_eip1108", "f7", t) }
func BenchmarkPrecompiledBn256ScalarMulEip1108(b *testing.B) {
	benchJSON("bn256ScalarMul_eip1108", "f7", b)
}

// Tests the sample inputs from the elliptic curve pairing check EIP 197.
func TestPrecompiledBn256Pairing(t *testing.T)      { testJSON("bn256Pairing", "08", t) }
func BenchmarkPrecompiledBn256Pairing(b *testing.B) { benchJSON("bn256Pairing", "08", b) }

func TestPrecompiledBn256PairingEip1108(t *testing.T) { testJSON("bn256Pairing_eip1108", "f8", t) }
func BenchmarkPrecompiledBn256PairingEip1108(b *testing.B) {
	benchJSON("bn256Pairing_eip1108", "f8", b)
}

func TestPrecompiledBlake2F(t *testing.T)      { testJSON("blake2F", "09", t) }
func BenchmarkPrecompiledBlake2F(b *testing.B) { benchJSON("blake2F", "09", b) }

func TestPrecompiledEcrecover(t *testing.T) { testJSON("ecRecover", "01", t) }

// Failure tests
func TestPrecompiledBlake2FFailure(t *testing.T) { testJSONFail("blake2F", "09", t) }

func testJSON(name, addr string, t *testing.T) {
	tests, err := loadJSON(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		testPrecompiled(addr, test, t)
	}
}

func testJSONFail(name, addr string, t *testing.T) {
	tests, err := loadJSONFail(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		testPrecompiledFailure(addr, test, t)
	}
}

func benchJSON(name, addr string, b *testing.B) {
	tests, err := loadJSON(name)
	if err != nil {
		b.Fatal(err)
	}
	for _, test := range tests {
		benchmarkPrecompiled(addr, test, b)
	}
}

func loadJSON(name string) ([]precompiledTest, error) {
	data, err := os.ReadFile(fmt.Sprintf("testdata/precompiles/%v.json", name))
	if err != nil {
		return nil, err
	}
	var testcases []precompiledTest
	err = json.Unmarshal(data, &testcases)
	return testcases, err
}

func loadJSONFail(name string) ([]precompiledFailureTest, error) {
	data, err := os.ReadFile(fmt.Sprintf("testdata/precompiles/fail-%v.json", name))
	if err != nil {
		return nil, err
	}
	var testcases []precompiledFailureTest
	err = json.Unmarshal(data, &testcases)
	return testcases, err
}

func TestAsDelegate(t *testing.T) {
	// Mock addresses
	parentCallerAddress := common.HexToAddress("0x01")
	objectAddress := common.HexToAddress("0x03")

	// Create a parent contract to act as the caller
	parentContract := NewContract(AccountRef(parentCallerAddress), AccountRef(parentCallerAddress), big.NewInt(2000), 5000)

	// Create a child contract, which will be turned into a delegate
	childContract := NewContract(parentContract, AccountRef(objectAddress), big.NewInt(2000), 5000)

	// Call AsDelegate on the child contract
	delegatedContract := childContract.AsDelegate()

	// Perform your test assertions
	assert.True(t, delegatedContract.DelegateCall, "Contract should be in delegate call mode")
	assert.Equal(t, parentContract.CallerAddress, delegatedContract.CallerAddress, "Caller address should match parent contract caller address")
	assert.Equal(t, parentContract.value, delegatedContract.value, "Value should match parent contract value")
}

func TestValidJumpdest(t *testing.T) {
	// Example bytecode: PUSH1 0x02 JUMPDEST STOP
	code := []byte{0x60, 0x02, 0x5b, 0x00}

	contract := &Contract{
		Code: code,
	}

	// Test a valid jump destination (position of JUMPDEST opcode)
	validDest := uint256.NewInt(2)
	assert.True(t, contract.validJumpdest(validDest), "Expected valid jump destination")

	// Test an invalid jump destination (within PUSH1 data)
	invalidDest := uint256.NewInt(1)
	assert.False(t, contract.validJumpdest(invalidDest), "Expected invalid jump destination due to being within PUSH data")

	// Test an invalid jump destination (non-existent opcode)
	nonExistentDest := uint256.NewInt(100)
	assert.False(t, contract.validJumpdest(nonExistentDest), "Expected invalid jump destination due to non-existent opcode")

	// Test a non-JUMPDEST opcode (STOP opcode)
	nonJumpdestOpcode := uint256.NewInt(3)
	assert.False(t, contract.validJumpdest(nonJumpdestOpcode), "Expected invalid jump destination due to non-JUMPDEST opcode")

	// Test edge cases
	// Destination right at the start of the code
	startOfCode := uint256.NewInt(0)
	assert.False(t, contract.validJumpdest(startOfCode), "Expected invalid jump destination at the start of the code")

	// Destination right at the end of the code
	endOfCode := uint256.NewInt(uint64(len(code) - 1))
	assert.False(t, contract.validJumpdest(endOfCode), "Expected invalid jump destination at the end of the code")
}

func TestIsCode(t *testing.T) {
	// Example bytecode: PUSH1 0x02 JUMPDEST STOP
	code := []byte{0x60, 0x02, 0x5b, 0x00}

	contract := &Contract{
		Code: code,
	}

	// Test when analysis is not set
	assert.False(t, contract.isCode(1), "Position 1 should not be valid code")
	assert.True(t, contract.isCode(2), "Position 2 should be valid code")

	// Test that analysis is now set after calling isCode
	assert.NotNil(t, contract.analysis, "Analysis should be set after calling isCode")
}

func setupContract() *Contract {
	return &Contract{
		CallerAddress: common.HexToAddress("0x01"),
		value:         big.NewInt(1000),
		Code:          []byte{0x60, 0x02, 0x5b, 0x00}, // Example bytecode
		CodeHash:      common.HexToHash("somehash"),
		CodeAddr:      new(common.Address),
	}
}

func TestGetOp(t *testing.T) {
	contract := setupContract()
	assert.Equal(t, OpCode(0x60), contract.GetOp(0), "Expected OpCode at position 0 to match")
	assert.Equal(t, OpCode(0x5b), contract.GetOp(2), "Expected OpCode at position 2 to match")
}

func TestGetByte(t *testing.T) {
	contract := setupContract()
	assert.Equal(t, byte(0x60), contract.GetByte(0), "Expected byte at position 0 to match")
	assert.Equal(t, byte(0x00), contract.GetByte(3), "Expected byte at position 3 to match")
	assert.Equal(t, byte(0x00), contract.GetByte(10), "Expected byte at out of bounds position to be 0")
}

func TestCaller(t *testing.T) {
	contract := setupContract()
	assert.Equal(t, common.HexToAddress("0x01"), contract.Caller(), "Expected caller address to match")
}

func TestValue(t *testing.T) {
	contract := setupContract()
	assert.Equal(t, big.NewInt(1000), contract.Value(), "Expected value to match")
}

func TestSetCode(t *testing.T) {
	contract := setupContract()
	newCode := []byte{0x01, 0x02}
	newHash := common.HexToHash("newhash")
	contract.SetCode(newHash, newCode)

	assert.Equal(t, newCode, contract.Code, "Expected code to be updated")
	assert.Equal(t, newHash, contract.CodeHash, "Expected code hash to be updated")
}

func TestSetCallCode(t *testing.T) {
	contract := setupContract()
	newCode := []byte{0x03, 0x04}
	newHash := common.HexToHash("newerhash")
	newAddr := common.HexToAddress("0x02")
	contract.SetCallCode(&newAddr, newHash, newCode)

	assert.Equal(t, newCode, contract.Code, "Expected code to be updated")
	assert.Equal(t, newHash, contract.CodeHash, "Expected codehash to be updated")
	assert.Equal(t, &newAddr, contract.CodeAddr, "Expected code address to be updated")
}
