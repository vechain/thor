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

package tracers_test

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/tracers/logger"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"

	// Force-load the tracer engines to trigger registration
	_ "github.com/vechain/thor/tracers/js"
	_ "github.com/vechain/thor/tracers/native"
)

type callLog struct {
	Address common.Address `json:"address"`
	Topics  []common.Hash  `json:"topics"`
	Data    hexutil.Bytes  `json:"data"`
}

type callFrame struct {
	Type    string                `json:"type"`
	From    thor.Address          `json:"from"`
	To      thor.Address          `json:"to,omitempty"`
	Value   *math.HexOrDecimal256 `json:"value,omitempty"`
	Gas     math.HexOrDecimal64   `json:"gas"`
	GasUsed math.HexOrDecimal64   `json:"gasUsed"`
	Input   hexutil.Bytes         `json:"input"`
	Output  hexutil.Bytes         `json:"output,omitempty"`
	Error   string                `json:"error,omitempty"`
	Calls   []callFrame           `json:"calls,omitempty"`
	Logs    []callLog             `json:"logs,omitempty"`
}

type clause struct {
	To    *thor.Address         `json:"to,omitempty"`
	Value *math.HexOrDecimal256 `json:"value"`
	Data  hexutil.Bytes         `json:"data"`
}

type account struct {
	Balance *math.HexOrDecimal256        `json:"balance"`
	Energy  *math.HexOrDecimal256        `json:"energy"`
	Code    hexutil.Bytes                `json:"code"`
	Storage map[common.Hash]thor.Bytes32 `json:"storage"`
}

type context struct {
	BlockID     thor.Bytes32        `json:"blockID"`
	BlockTime   uint64              `json:"blockTime"`
	Beneficiary thor.Address        `json:"beneficiary"`
	TxOrigin    thor.Address        `json:"txOrigin"`
	ClauseIndex uint32              `json:"clauseIndex"`
	TxID        thor.Bytes32        `json:"txID"`
	Gas         math.HexOrDecimal64 `json:"gas"`
}

type traceTest struct {
	Clause  clause                     `json:"clause"`
	Context context                    `json:"context"`
	State   map[common.Address]account `json:"state,omitempty"`
	Config  json.RawMessage            `json:"config"`
}

type callTest struct {
	traceTest
	Calls callFrame `json:"calls,omitempty"`
}

type diffState struct {
	Pre  map[common.Address]account `json:"pre"`
	Post map[common.Address]account `json:"post"`
}

type prestateTest struct {
	traceTest
	diffState
}

func RunTracerTest(t *testing.T, data *traceTest, tracerName string) json.RawMessage {
	db := muxdb.NewMem()
	gene, _, _, err := genesis.NewTestnet().Build(state.NewStater(db))
	if err != nil {
		t.Fatal(err)
	}

	repo, _ := chain.NewRepository(db, gene)
	st := state.New(db, gene.Header().StateRoot(), 0, 0, 0)
	chain := repo.NewChain(gene.Header().ID())

	for addr, account := range data.State {
		st.SetBalance(thor.Address(addr), (*big.Int)(account.Balance))
		st.SetEnergy(thor.Address(addr), (*big.Int)(account.Energy), data.Context.BlockTime)
		if len(account.Code) > 0 {
			st.SetCode(thor.Address(addr), account.Code)
		}
		for k, v := range account.Storage {
			st.SetStorage(thor.Address(addr), thor.Bytes32(k), v)
		}
	}

	rt := runtime.New(chain, st, &xenv.BlockContext{
		Number:      block.Number(data.Context.BlockID),
		Time:        data.Context.BlockTime,
		Beneficiary: data.Context.Beneficiary,
	}, thor.GetForkConfig(gene.Header().ID()))

	var tr tracers.Tracer
	if len(tracerName) > 0 {
		tr, err = tracers.DefaultDirectory.New(tracerName, data.Config, false)
		assert.Nil(t, err)
	} else {
		cfg, _ := json.Marshal(logger.Config{
			EnableMemory:     true,
			EnableReturnData: true,
		})
		tr, _ = logger.NewStructLogger(cfg)
	}

	tr.SetContext(&tracers.Context{
		BlockTime: rt.Context().Time,
		State:     rt.State(),
	})
	rt.SetVMConfig(vm.Config{Tracer: tr})

	clause := tx.NewClause(data.Clause.To).WithValue((*big.Int)(data.Clause.Value)).WithData(data.Clause.Data)
	exec, _ := rt.PrepareClause(clause, data.Context.ClauseIndex, uint64(data.Context.Gas), &xenv.TransactionContext{
		Origin: data.Context.TxOrigin,
		ID:     data.Context.TxID,
	})
	tr.CaptureClauseStart(uint64(data.Context.Gas))
	output, _, err := exec()
	assert.Nil(t, err)

	leftOverGas := output.LeftOverGas
	gasUsed := uint64(data.Context.Gas) - leftOverGas
	refund := gasUsed / 2
	if refund > output.RefundGas {
		refund = output.RefundGas
	}
	leftOverGas += refund

	tr.CaptureClauseEnd(leftOverGas)
	result, err := tr.GetResult()
	assert.Nil(t, err)
	return result
}

func TestNewTracer(t *testing.T) {
	_, err := tracers.DefaultDirectory.New("callTracer", nil, false)
	assert.Nil(t, err)
}

func TestAllTracers(t *testing.T) {
	var testData callTest
	if blob, err := os.ReadFile("testdata/calls.json"); err != nil {
		t.Fatalf("failed to read testcase: %v", err)
	} else if err := json.Unmarshal(blob, &testData); err != nil {
		t.Fatalf("failed to parse testcase: %v", err)
	}

	RunTracerTest(t, &testData.traceTest, "")
	RunTracerTest(t, &testData.traceTest, "4byteTracer")
	RunTracerTest(t, &testData.traceTest, "unigram")
	RunTracerTest(t, &testData.traceTest, "bigram")
	RunTracerTest(t, &testData.traceTest, "trigram")
	RunTracerTest(t, &testData.traceTest, "evmdis")
	RunTracerTest(t, &testData.traceTest, "opcount")
}

func TestCallTracers(t *testing.T) {
	files, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		f := file
		t.Run(strings.TrimSuffix(f.Name(), ".json"), func(t *testing.T) {
			var testData callTest

			if blob, err := os.ReadFile(filepath.Join("testdata", file.Name())); err != nil {
				t.Fatalf("failed to read testcase: %v", err)
			} else if err := json.Unmarshal(blob, &testData); err != nil {
				t.Fatalf("failed to parse testcase: %v", err)
			}

			result := RunTracerTest(t, &testData.traceTest, "callTracer")
			var got callFrame
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, testData.Calls, got)

			result = RunTracerTest(t, &testData.traceTest, "prestateTracer")
			type prestate map[common.Address]account
			var pre prestate
			if err := json.Unmarshal(result, &pre); err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, prestate(testData.State), pre)
		})

	}
}

func TestPreStateTracers(t *testing.T) {
	files, err := os.ReadDir("testdata/prestate_diff")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		f := file
		t.Run(strings.TrimSuffix(f.Name(), ".json"), func(t *testing.T) {
			var testData prestateTest

			if blob, err := os.ReadFile(filepath.Join("testdata/prestate_diff", file.Name())); err != nil {
				t.Fatalf("failed to read testcase: %v", err)
			} else if err := json.Unmarshal(blob, &testData); err != nil {
				t.Fatalf("failed to parse testcase: %v", err)
			}

			result := RunTracerTest(t, &testData.traceTest, "prestateTracer")
			var got diffState
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, testData.diffState, got)
		})

	}
}

func TestInternals(t *testing.T) {
	var (
		to     = thor.MustParseAddress("0x00000000000000000000000000000000deadbeef")
		origin = thor.MustParseAddress("0x000000000000000000000000000000000000feed")
	)
	mkTracer := func(name string, cfg json.RawMessage) tracers.Tracer {
		tr, err := tracers.DefaultDirectory.New(name, cfg, false)
		if err != nil {
			t.Fatalf("failed to create call tracer: %v", err)
		}
		return tr
	}

	for _, tc := range []struct {
		name   string
		code   []byte
		tracer tracers.Tracer
		want   string
	}{
		{
			// TestZeroValueToNotExitCall tests the calltracer(s) on the following:
			// Tx to A, A calls B with zero value. B does not already exist.
			// Expected: that enter/exit is invoked and the inner call is shown in the result
			name: "ZeroValueToNotExitCall",
			code: []byte{
				byte(vm.PUSH1), 0x0, byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1), // in and outs zero
				byte(vm.DUP1), byte(vm.PUSH1), 0xff, byte(vm.GAS), // value=0,address=0xff, gas=GAS
				byte(vm.CALL),
			},
			tracer: mkTracer("callTracer", nil),
			want:   `{"from":"0x000000000000000000000000000000000000feed","gas":"0x13880","gasUsed":"0x54d8","to":"0x00000000000000000000000000000000deadbeef","input":"0x","calls":[{"from":"0x00000000000000000000000000000000deadbeef","gas":"0xe01a","gasUsed":"0x0","to":"0x00000000000000000000000000000000000000ff","input":"0x","value":"0x0","type":"CALL"}],"value":"0x0","type":"CALL"}`,
		},
		{
			name:   "Stack depletion in LOG0",
			code:   []byte{byte(vm.LOG3)},
			tracer: mkTracer("callTracer", json.RawMessage(`{ "withLog": true }`)),
			want:   `{"from":"0x000000000000000000000000000000000000feed","gas":"0x13880","gasUsed":"0x13880","to":"0x00000000000000000000000000000000deadbeef","input":"0x","error":"stack underflow (0 \u003c=\u003e 5)","value":"0x0","type":"CALL"}`,
		},
		{
			name: "Mem expansion in LOG0",
			code: []byte{
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH1), 0x0,
				byte(vm.MSTORE),
				byte(vm.PUSH1), 0xff,
				byte(vm.PUSH1), 0x0,
				byte(vm.LOG0),
			},
			tracer: mkTracer("callTracer", json.RawMessage(`{ "withLog": true }`)),
			want:   `{"from":"0x000000000000000000000000000000000000feed","gas":"0x13880","gasUsed":"0x5b9e","to":"0x00000000000000000000000000000000deadbeef","input":"0x","logs":[{"address":"0x00000000000000000000000000000000deadbeef","topics":[],"data":"0x000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"}],"value":"0x0","type":"CALL"}`,
		},
		{
			// Leads to OOM on the prestate tracer
			name: "Prestate-tracer - CREATE2 OOM",
			code: []byte{
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH1), 0x0,
				byte(vm.MSTORE),
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH5), 0xff, 0xff, 0xff, 0xff, 0xff,
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH1), 0x0,
				byte(vm.CREATE2),
				byte(vm.PUSH1), 0xff,
				byte(vm.PUSH1), 0x0,
				byte(vm.LOG0),
			},
			tracer: mkTracer("prestateTracer", nil),
			// Here adds energy to prestate test cases, in ethereum prestate tracer adds tx fee back to origin's balance, we don't(clause level)
			want: `{"0x0000000000000000000000000000000000000000":{"balance":"0x0","energy":"0x0"},"0x000000000000000000000000000000000000feed":{"balance":"0x1c6bf52634000","energy":"0x0"},"0x00000000000000000000000000000000deadbeef":{"balance":"0x0","energy":"0x0","code":"0x6001600052600164ffffffffff60016000f560ff6000a0"}}`,
		},
		{
			// CREATE2 which requires padding memory by prestate tracer
			name: "Prestate-tracer - CREATE2 Memory padding",
			code: []byte{
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH1), 0x0,
				byte(vm.MSTORE),
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH1), 0xff,
				byte(vm.PUSH1), 0x1,
				byte(vm.PUSH1), 0x0,
				byte(vm.CREATE2),
				byte(vm.PUSH1), 0xff,
				byte(vm.PUSH1), 0x0,
				byte(vm.LOG0),
			},
			tracer: mkTracer("prestateTracer", nil),
			// Here adds energy to prestate test cases, in ethereum prestate tracer adds tx fee back to origin's balance, we don't(clause level)
			want: `{"0x0000000000000000000000000000000000000000":{"balance":"0x0","energy":"0x0"},"0x000000000000000000000000000000000000feed":{"balance":"0x1c6bf52634000","energy":"0x0"},"0x00000000000000000000000000000000deadbeef":{"balance":"0x0","energy":"0x0","code":"0x6001600052600160ff60016000f560ff6000a0"},"0x91ff9a805d36f54e3e272e230f3e3f5c1b330804":{"balance":"0x0","energy":"0x0"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := muxdb.NewMem()
			gene, _, _, err := genesis.NewTestnet().Build(state.NewStater(db))
			if err != nil {
				t.Fatal(err)
			}

			repo, _ := chain.NewRepository(db, gene)
			st := state.New(db, gene.Header().StateRoot(), 0, 0, 0)
			chain := repo.NewChain(gene.Header().ID())

			st.SetCode(to, tc.code)
			st.SetBalance(origin, big.NewInt(500000000000000))

			rt := runtime.New(chain, st, &xenv.BlockContext{
				Number:      8000000,
				Time:        5,
				Beneficiary: thor.Address{},
				GasLimit:    6000000,
			}, thor.GetForkConfig(gene.Header().ID()))

			tr := tc.tracer

			tr.SetContext(&tracers.Context{
				BlockTime: rt.Context().Time,
				State:     rt.State(),
			})
			rt.SetVMConfig(vm.Config{Tracer: tr})

			gas := uint64(80000)
			clause := tx.NewClause(&to).WithValue((*big.Int)(big.NewInt(0)))
			// to remain the same with testcases from ethereum, here deduct intrinsic gas since ethereum captures gas including intrinsic and we don't
			// we are capturing at clause level
			exec, _ := rt.PrepareClause(clause, 0, gas-21000, &xenv.TransactionContext{
				Origin:   origin,
				GasPrice: big.NewInt(0),
			})

			tr.CaptureClauseStart(gas)
			output, _, err := exec()
			if err != nil {
				t.Fatalf("test %v: failed to execute: %v", tc.name, err)
			}

			leftOverGas := output.LeftOverGas
			gasUsed := gas - leftOverGas
			refund := gasUsed / 2
			if refund > output.RefundGas {
				refund = output.RefundGas
			}
			leftOverGas += refund

			tr.CaptureClauseEnd(leftOverGas)
			res, err := tc.tracer.GetResult()
			if err != nil {
				t.Fatalf("test %v: failed to retrieve trace result: %v", tc.name, err)
			}
			if string(res) != tc.want {
				t.Errorf("test %v: trace mismatch\n have: %v\n want: %v\n", tc.name, string(res), tc.want)
			}
		})
	}
}
