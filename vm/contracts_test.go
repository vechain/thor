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
	"maps"
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

// allPrecompiles contains the real Osaka precompiles plus historical/repriced
// variants at fake addresses for regression testing. Real addresses always
// reflect production; fake addresses are test-only historical variants.
var allPrecompiles = func() map[common.Address]PrecompiledContract {
	m := make(map[common.Address]PrecompiledContract)
	maps.Copy(m, PrecompiledContractsOsaka)

	// Historical variants at fake addresses — not production addresses for latest fork
	m[common.BytesToAddress([]byte{0xb5})] = &bigModExp{eip2565: false}               // EIP-198 original
	m[common.BytesToAddress([]byte{0xf5})] = &bigModExp{eip2565: true}                // EIP-2565
	m[common.BytesToAddress([]byte{0xe5})] = &bigModExp{eip2565: true, eip7823: true} // EIP-2565 + EIP-7823
	m[common.BytesToAddress([]byte{0xf9})] = &bigModExp{eip2565: true, eip7883: true} // EIP-2565 + EIP-7883
	m[common.BytesToAddress([]byte{0xb6})] = &bn256Add{eip1108: false}                // bn256Add original
	m[common.BytesToAddress([]byte{0xf6})] = &bn256Add{eip1108: true}                 // bn256Add EIP-1108
	m[common.BytesToAddress([]byte{0xb7})] = &bn256ScalarMul{eip1108: false}          // bn256ScalarMul original
	m[common.BytesToAddress([]byte{0xf7})] = &bn256ScalarMul{eip1108: true}           // bn256ScalarMul EIP-1108
	m[common.BytesToAddress([]byte{0xb8})] = &bn256Pairing{eip1108: false}            // bn256Pairing original
	m[common.BytesToAddress([]byte{0xf8})] = &bn256Pairing{eip1108: true}             // bn256Pairing EIP-1108
	return m
}()

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
		for bench.Loop() {
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
		// Check if it is correct
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
func TestPrecompiledModExp(t *testing.T)      { testJSON("modexp", "b5", t) }
func BenchmarkPrecompiledModExp(b *testing.B) { benchJSON("modexp", "b5", b) }

func TestPrecompiledModExpEip2565(t *testing.T)      { testJSON("modexp_eip2565", "f5", t) }
func BenchmarkPrecompiledModExpEip2565(b *testing.B) { benchJSON("modexp_eip2565", "f5", b) }

// Tests the ModExp precompile with EIP-7883 gas repricing (Osaka/Fusaka).
func TestPrecompiledModExpEip7883(t *testing.T)      { testJSON("modexp_eip7883", "f9", t) }
func BenchmarkPrecompiledModExpEip7883(b *testing.B) { benchJSON("modexp_eip7883", "f9", b) }

// EIP-7823 tests: existing modexp tests should still pass with eip7823 enabled
// (all standard test vectors use inputs within the 1024-byte limit)
func TestPrecompiledModExpEip7823(t *testing.T)      { testJSON("modexp_eip2565", "e5", t) }
func BenchmarkPrecompiledModExpEip7823(b *testing.B) { benchJSON("modexp_eip2565", "e5", b) }

func TestPrecompiledModExpEip7823Failure(t *testing.T) {
	testJSONFail("modexp_eip7823", "e5", t)
}

func TestPrecompiledModExpEip7823_LengthOverflow(t *testing.T) {
	p := allPrecompiles[common.HexToAddress("e5")]

	// base length overflows uint64 (33 bytes = 264 bits > 64 bits)
	// This triggers the inputLenOverflow check from the PR #32363 fix.
	// 32-byte length field with value > 2^64
	input := common.Hex2Bytes(
		"0000000000000000000000000000000000000000000000010000000000000001" + // baseLen = 2^64 + 1 (overflows uint64)
			"0000000000000000000000000000000000000000000000000000000000000001" + // expLen = 1
			"0000000000000000000000000000000000000000000000000000000000000001", // modLen = 1
	)
	gas := p.RequiredGas(input)
	contract := NewContract(AccountRef(common.HexToAddress("1337")),
		nil, new(big.Int), gas)
	_, err := RunPrecompiledContract(p, input, contract)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded 1024 bytes")
}

// Tests the sample inputs from the elliptic curve addition EIP 213.
func TestPrecompiledBn256Add(t *testing.T)      { testJSON("bn256Add", "b6", t) }
func BenchmarkPrecompiledBn256Add(b *testing.B) { benchJSON("bn256Add", "b6", b) }

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
func TestPrecompiledBn256ScalarMul(t *testing.T)      { testJSON("bn256ScalarMul", "b7", t) }
func BenchmarkPrecompiledBn256ScalarMul(b *testing.B) { benchJSON("bn256ScalarMul", "b7", b) }

func TestPrecompiledBn256ScalarMulEip1108(t *testing.T) { testJSON("bn256ScalarMul_eip1108", "f7", t) }
func BenchmarkPrecompiledBn256ScalarMulEip1108(b *testing.B) {
	benchJSON("bn256ScalarMul_eip1108", "f7", b)
}

// Tests the sample inputs from the elliptic curve pairing check EIP 197.
func TestPrecompiledBn256Pairing(t *testing.T)      { testJSON("bn256Pairing", "b8", t) }
func BenchmarkPrecompiledBn256Pairing(b *testing.B) { benchJSON("bn256Pairing", "b8", b) }

func TestPrecompiledBn256PairingEip1108(t *testing.T) { testJSON("bn256Pairing_eip1108", "f8", t) }
func BenchmarkPrecompiledBn256PairingEip1108(b *testing.B) {
	benchJSON("bn256Pairing_eip1108", "f8", b)
}

func TestPrecompiledBlake2F(t *testing.T)      { testJSON("blake2F", "09", t) }
func BenchmarkPrecompiledBlake2F(b *testing.B) { benchJSON("blake2F", "09", b) }

func TestPrecompiledEcrecover(t *testing.T) { testJSON("ecRecover", "01", t) }

// Failure tests
func TestPrecompiledBlake2FFailure(t *testing.T) { testJSONFail("blake2F", "09", t) }

// Tests the sample inputs from EIP-2537: BLS12-381 curve operations (Prague).
func TestPrecompiledBLS12381G1Add(t *testing.T)      { testJSON("blsG1Add", "0b", t) }
func BenchmarkPrecompiledBLS12381G1Add(b *testing.B) { benchJSON("blsG1Add", "0b", b) }

func TestPrecompiledBLS12381G1MultiExp(t *testing.T)      { testJSON("blsG1MultiExp", "0c", t) }
func BenchmarkPrecompiledBLS12381G1MultiExp(b *testing.B) { benchJSON("blsG1MultiExp", "0c", b) }

func TestPrecompiledBLS12381G2Add(t *testing.T)      { testJSON("blsG2Add", "0d", t) }
func BenchmarkPrecompiledBLS12381G2Add(b *testing.B) { benchJSON("blsG2Add", "0d", b) }

func TestPrecompiledBLS12381G2MultiExp(t *testing.T)      { testJSON("blsG2MultiExp", "0e", t) }
func BenchmarkPrecompiledBLS12381G2MultiExp(b *testing.B) { benchJSON("blsG2MultiExp", "0e", b) }

func TestPrecompiledBLS12381Pairing(t *testing.T)      { testJSON("blsPairing", "0f", t) }
func BenchmarkPrecompiledBLS12381Pairing(b *testing.B) { benchJSON("blsPairing", "0f", b) }

func TestPrecompiledBLS12381MapG1(t *testing.T)      { testJSON("blsMapG1", "10", t) }
func BenchmarkPrecompiledBLS12381MapG1(b *testing.B) { benchJSON("blsMapG1", "10", b) }

func TestPrecompiledBLS12381MapG2(t *testing.T)      { testJSON("blsMapG2", "11", t) }
func BenchmarkPrecompiledBLS12381MapG2(b *testing.B) { benchJSON("blsMapG2", "11", b) }

// BLS12-381 failure tests
func TestPrecompiledBLS12381G1AddFail(t *testing.T)      { testJSONFail("blsG1Add", "0b", t) }
func TestPrecompiledBLS12381G1MultiExpFail(t *testing.T) { testJSONFail("blsG1MultiExp", "0c", t) }
func TestPrecompiledBLS12381G2AddFail(t *testing.T)      { testJSONFail("blsG2Add", "0d", t) }
func TestPrecompiledBLS12381G2MultiExpFail(t *testing.T) { testJSONFail("blsG2MultiExp", "0e", t) }
func TestPrecompiledBLS12381PairingFail(t *testing.T)    { testJSONFail("blsPairing", "0f", t) }
func TestPrecompiledBLS12381MapG1Fail(t *testing.T)      { testJSONFail("blsMapG1", "10", t) }
func TestPrecompiledBLS12381MapG2Fail(t *testing.T)      { testJSONFail("blsMapG2", "11", t) }

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

// Benchmarks the sample inputs from the P256VERIFY precompile.
func BenchmarkPrecompiledP256Verify(bench *testing.B) {
	t := precompiledTest{
		Input:    "4cee90eb86eaa050036147a12d49004b6b9c72bd725d39d4785011fe190f0b4da73bd4903f0ce3b639bbbf6e8e80d16931ff4bcf5993d58468e8fb19086e8cac36dbcd03009df8c59286b162af3bd7fcc0450c9aa81be5d10d312af6c66b1d604aebd3099c618202fcfe16ae7770b0c49ab5eadf74b754204a3bb6060e44eff37618b065f9832de4ca6ca971a7a1adc826d0f7c00181a5fb2ddf79ae00b4e10e",
		Expected: "0000000000000000000000000000000000000000000000000000000000000001",
		Name:     "p256Verify",
	}
	benchmarkPrecompiled("0b", t, bench)
}

func TestPrecompiledP256Verify(t *testing.T) { testJSON("p256Verify", "0100", t) }

func TestPrecompiledP256Verify_PointAtInfinity(t *testing.T) {
	p := allPrecompiles[common.HexToAddress("0100")]

	// EIP-7951: point at infinity (x == 0, y == 0) must return nil (failure), not panic.
	input := make([]byte, 160)
	// hash: bytes [0:32]   — zeros
	// r:    bytes [32:64]  — zeros
	// s:    bytes [64:96]  — zeros
	// x:    bytes [96:128] — zeros (point at infinity)
	// y:    bytes [128:160]— zeros (point at infinity)

	res, err := p.Run(input)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

// sentinelByte marks bytes just past an input window / spare backing
// capacity, so a precompile writing there (in-place append, in-place
// mutation) flips it and gets caught.
const sentinelByte = 0xAA

// TestEcrecoverDoesNotWriteInput guards an invariant introduced by
// memory.GetPtr: the call opcodes now pass a zero-copy view of the caller's
// EVM memory straight into precompiles, so a precompile's input IS the
// caller's memory and must never be written through. ecrecover
// (vm/contracts.go) copies the signature onto a fresh [65]byte array before
// calling crypto.Ecrecover, so it never touches its input slice.
//
// This drives a STATICCALL to 0x01 through the real opcode path (rather than
// calling ecrecover.Run directly) so the GetPtr aliasing is actually
// exercised, with a 128-byte input crafted to reach ecrecover.Run's
// signature-construction step: (hash=0, v=27 => v-27=0, r=1, s=1) satisfies
// the all-zero check on input[32:63] and ValidateSignatureValues(0, 1, 1,
// false), regardless of what crypto.Ecrecover itself returns.
//
// The precompile fork table doesn't matter here — 0x01 resolves to the same
// &ecrecover{} in every table (vm/contracts.go) — so this only needs to run
// once, against Osaka.
func TestEcrecoverDoesNotWriteInput(t *testing.T) {
	chainCfg := &ChainConfig{OsakaBlock: big.NewInt(0)}
	evm := NewEVM(Context{BlockNumber: big.NewInt(0)}, NoopStateDB{}, chainCfg, Config{})
	if _, ok := evm.precompile(common.BytesToAddress([]byte{1})); !ok {
		t.Fatal("expected a precompile at 0x01")
	}

	contract := NewContract(AccountRef(common.Address{}), AccountRef(common.Address{}), big.NewInt(0), 100000)

	memory := NewMemory()
	// 256 bytes gives the 128-byte input spare capacity past byte 128, so
	// this remains a regression check: if ecrecover.Run ever reverts to
	// writing through its input slice (e.g. append(input[64:128], v)), that
	// write lands in-place here instead of on a reallocated backing array,
	// and the sentinel below catches it.
	memory.Resize(256)

	input := make([]byte, 128)
	input[63] = 27
	input[95] = 1  // r = 1
	input[127] = 1 // s = 1
	memory.Set(0, 128, input)
	memory.store[128] = sentinelByte

	stack := newstack()
	// STATICCALL(gas, addr=0x01, inOffset=0, inSize=128, retOffset=0, retSize=0)
	stack.push(uint256.NewInt(0))                                                   // retSize
	stack.push(uint256.NewInt(0))                                                   // retOffset
	stack.push(uint256.NewInt(128))                                                 // inSize
	stack.push(uint256.NewInt(0))                                                   // inOffset
	stack.push(new(uint256.Int).SetBytes(common.BytesToAddress([]byte{1}).Bytes())) // addr = 0x01
	stack.push(uint256.NewInt(100000))                                              // gas (popped & discarded by opStaticCall)
	evm.callGasTemp = 100000

	pc := uint64(0)
	if _, err := opStaticCall(&pc, evm, contract, memory, stack); err != nil {
		t.Fatal(err)
	}

	if got := memory.store[128]; got != sentinelByte {
		t.Errorf("ecrecover wrote past its 128-byte input window: got %#x, want sentinel %#x", got, byte(sentinelByte))
	}
}

// precompileVectors maps a precompile's test address (in the same short hex
// form used by testJSON elsewhere in this file) to a testdata/precompiles
// JSON file supplying real success-path inputs. Precompiles without a vector
// file fall back to genericFillerInput.
var precompileVectors = func() map[common.Address]string {
	names := map[string]string{
		"01":   "ecRecover",
		"05":   "modexp_eip7883", // Osaka: eip2565+eip7823+eip7883
		"06":   "bn256Add_eip1108",
		"07":   "bn256ScalarMul_eip1108",
		"08":   "bn256Pairing_eip1108",
		"09":   "blake2F",
		"0b":   "blsG1Add",
		"0c":   "blsG1MultiExp",
		"0d":   "blsG2Add",
		"0e":   "blsG2MultiExp",
		"0f":   "blsPairing",
		"10":   "blsMapG1",
		"11":   "blsMapG2",
		"0100": "p256Verify",
		"b5":   "modexp",
		"f5":   "modexp_eip2565",
		"e5":   "modexp_eip2565", // eip7823 variant; vectors are all within the 1024-byte limit
		"f9":   "modexp_eip7883",
		"b6":   "bn256Add",
		"f6":   "bn256Add_eip1108",
		"b7":   "bn256ScalarMul",
		"f7":   "bn256ScalarMul_eip1108",
		"b8":   "bn256Pairing",
		"f8":   "bn256Pairing_eip1108",
	}
	m := make(map[common.Address]string, len(names))
	for addr, name := range names {
		m[common.HexToAddress(addr)] = name
	}
	return m
}()

// genericFillerInput covers precompiles with no testdata/precompiles vector
// file (sha256, ripemd160, identity: all take arbitrary-length input and
// always succeed, so no bespoke vector is needed to reach their write path).
var genericFillerInput = func() []byte {
	b := make([]byte, 37)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}()

// precompileInputs returns the set of inputs to exercise for addr, preferring
// real success-path vectors (see precompileVectors) over the generic filler.
func precompileInputs(t *testing.T, addr common.Address) [][]byte {
	t.Helper()
	name, ok := precompileVectors[addr]
	if !ok {
		return [][]byte{genericFillerInput}
	}
	tests, err := loadJSON(name)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([][]byte, len(tests))
	for i, test := range tests {
		inputs[i] = common.Hex2Bytes(test.Input)
	}
	return inputs
}

// TestPrecompilesDoNotWriteInput sweeps every entry in allPrecompiles and
// asserts none of them write through their input.
//
// This matters because the call opcodes now pass memory.GetPtr's zero-copy
// view straight into precompiles: a precompile's input IS the caller's own
// EVM memory, so a write there corrupts the caller's memory and can produce
// a consensus divergence between nodes. The precompiles present at the time
// of this change have already been audited (see TestEcrecoverDoesNotWriteInput
// and TestIdentityDoesNotWriteInput); this test's job is to catch a *future*
// precompile that regresses the invariant.
//
// Each input is copied into a backing array with spare capacity past the
// input window, filled with sentinelByte, so an in-place write (e.g. an
// append that happens to fit within cap()) corrupts bytes this test can see,
// instead of silently reallocating onto a private buffer.
func TestPrecompilesDoNotWriteInput(t *testing.T) {
	for addr, p := range allPrecompiles {
		t.Run(addr.Hex(), func(t *testing.T) {
			for _, in := range precompileInputs(t, addr) {
				original := append([]byte(nil), in...)

				buf := make([]byte, len(in)+64)
				copy(buf, in)
				for i := len(in); i < len(buf); i++ {
					buf[i] = sentinelByte
				}
				view := buf[:len(in)]

				// Return value / gas / error are covered by the existing
				// precompile-specific tests; this only checks for writes.
				_, _ = p.Run(view)

				if !bytes.Equal(view, original) {
					t.Errorf("precompile modified its input: got %x, want %x", view, original)
				}
				for i := len(in); i < len(buf); i++ {
					if buf[i] != sentinelByte {
						t.Errorf("precompile wrote past its input window at offset %d: got %#x, want sentinel %#x", i-len(in), buf[i], byte(sentinelByte))
						break
					}
				}
			}
		})
	}
}

// TestIdentityDoesNotWriteInput reproduces the aliasing hazard that
// memory.GetPtr introduces for the identity precompile (0x04): after
// opStaticCall's args become a zero-copy view of caller memory, dataCopy.Run
// returning `in` unmodified would mean the STATICCALL's return value — which
// the interpreter stores as this frame's returnData (vm/interpreter.go:222)
// for a later RETURNDATACOPY — still points into the caller's own memory. If
// the caller mutates that same memory region after the call (e.g. a later
// MSTORE reusing scratch space) but before reading returnData, it would
// observe the mutated bytes instead of the value returned at call time.
// common.CopyBytes in dataCopy.Run severs the alias.
//
// Note: inOffset and retOffset are deliberately non-overlapping (0 and 32).
// This is not about the overlapping memory.Set self-copy (Go's copy() is
// memmove-safe for any overlap) — it is about the returnData alias
// surviving the call and being exposed to writes the caller makes
// afterward.
func TestIdentityDoesNotWriteInput(t *testing.T) {
	evm := NewEVM(Context{}, NoopStateDB{}, &ChainConfig{OsakaBlock: big.NewInt(0)}, Config{})
	contract := NewContract(AccountRef(common.Address{}), AccountRef(common.Address{}), big.NewInt(0), 100000)

	memory := NewMemory()
	memory.Resize(64)
	original := make([]byte, 32)
	for i := range original {
		original[i] = byte(i)
	}
	memory.Set(0, 32, original)

	stack := newstack()
	// STATICCALL(gas, addr=0x04, inOffset=0, inSize=32, retOffset=32, retSize=32)
	stack.push(uint256.NewInt(32))                                                  // retSize
	stack.push(uint256.NewInt(32))                                                  // retOffset
	stack.push(uint256.NewInt(32))                                                  // inSize
	stack.push(uint256.NewInt(0))                                                   // inOffset
	stack.push(new(uint256.Int).SetBytes(common.BytesToAddress([]byte{4}).Bytes())) // addr = 0x04
	stack.push(uint256.NewInt(100000))                                              // gas (popped & discarded by opStaticCall)
	evm.callGasTemp = 100000

	pc := uint64(0)
	ret, err := opStaticCall(&pc, evm, contract, memory, stack)
	if err != nil {
		t.Fatal(err)
	}

	// simulate the interpreter capturing this opcode's result as returnData
	// (vm/interpreter.go:222 `in.returnData = res`)
	returnData := ret

	// caller reuses memory[0:32] for something else AFTER the call returns,
	// before a later RETURNDATACOPY would read returnData.
	memory.Set(0, 32, bytes.Repeat([]byte{0xFF}, 32))

	if !bytes.Equal(original, returnData) {
		t.Error("identity precompile output aliases caller memory: returnData observes a write made after the call")
	}
}
