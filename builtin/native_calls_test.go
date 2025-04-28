// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

var errReverted = vm.ErrExecutionReverted

var (
	thorChain *testchain.Chain
)

type ctest struct {
	rt         *runtime.Runtime
	abi        *abi.ABI
	to, caller thor.Address
}

type ccase struct {
	rt         *runtime.Runtime
	abi        *abi.ABI
	to, caller thor.Address
	name       string
	args       []any
	events     tx.Events
	provedWork *big.Int
	txID       thor.Bytes32
	blockRef   tx.BlockRef
	gasPayer   thor.Address
	expiration uint32
	value      *big.Int

	output *[]any
	vmerr  error
}

type TestTxDescription struct {
	t          *testing.T
	abi        *abi.ABI
	methodName string
	address    thor.Address
	acc        genesis.DevAccount
	args       []any
	duplicate  bool
	vet        *big.Int
}

func (c *ctest) Case(name string, args ...any) *ccase {
	return &ccase{
		rt:     c.rt,
		abi:    c.abi,
		to:     c.to,
		caller: c.caller,
		name:   name,
		args:   args,
	}
}

func (c *ccase) To(to thor.Address) *ccase {
	c.to = to
	return c
}

func (c *ccase) Caller(caller thor.Address) *ccase {
	c.caller = caller
	return c
}

func (c *ccase) Value(value *big.Int) *ccase {
	c.value = value
	return c
}

func (c *ccase) ProvedWork(provedWork *big.Int) *ccase {
	c.provedWork = provedWork
	return c
}

func (c *ccase) TxID(txID thor.Bytes32) *ccase {
	c.txID = txID
	return c
}

func (c *ccase) BlockRef(blockRef tx.BlockRef) *ccase {
	c.blockRef = blockRef
	return c
}

func (c *ccase) GasPayer(gasPayer thor.Address) *ccase {
	c.gasPayer = gasPayer
	return c
}

func (c *ccase) Expiration(expiration uint32) *ccase {
	c.expiration = expiration
	return c
}
func (c *ccase) ShouldVMError(err error) *ccase {
	c.vmerr = err
	return c
}

func (c *ccase) ShouldLog(events ...*tx.Event) *ccase {
	c.events = events
	return c
}

func (c *ccase) ShouldOutput(outputs ...any) *ccase {
	c.output = &outputs
	return c
}

func (c *ccase) Assert(t *testing.T) *ccase {
	method, ok := c.abi.MethodByName(c.name)
	assert.True(t, ok, "should have method")

	constant := method.Const()
	stage, err := c.rt.State().Stage(trie.Version{})
	assert.Nil(t, err, "should stage state")
	stateRoot := stage.Hash()

	data, err := method.EncodeInput(c.args...)
	assert.Nil(t, err, "should encode input")

	clause := tx.NewClause(&c.to).WithData(data)
	if c.value != nil {
		clause = clause.WithValue(c.value)
	}

	exec, _ := c.rt.PrepareClause(clause,
		0, math.MaxUint64, &xenv.TransactionContext{
			ID:         c.txID,
			Origin:     c.caller,
			GasPrice:   &big.Int{},
			GasPayer:   c.gasPayer,
			ProvedWork: c.provedWork,
			BlockRef:   c.blockRef,
			Expiration: c.expiration})
	vmout, _, err := exec()
	assert.Nil(t, err)
	if constant || vmout.VMErr != nil {
		stage, err := c.rt.State().Stage(trie.Version{})
		assert.Nil(t, err, "should stage state")
		newStateRoot := stage.Hash()
		assert.Equal(t, stateRoot, newStateRoot)
	}
	if c.vmerr != nil {
		assert.Equal(t, c.vmerr, vmout.VMErr)
	} else {
		assert.Nil(t, vmout.VMErr)
	}

	if c.output != nil {
		out, err := method.EncodeOutput((*c.output)...)
		assert.Nil(t, err, "should encode output")
		assert.Equal(t, out, vmout.Data, "should match output")
	}

	if len(c.events) > 0 {
		for _, ev := range c.events {
			found := func() bool {
				for _, outEv := range vmout.Events {
					if reflect.DeepEqual(ev, outEv) {
						return true
					}
				}
				return false
			}()
			assert.True(t, found, "event should appear")
		}
	}

	c.output = nil
	c.vmerr = nil
	c.events = nil

	return c
}

func buildGenesis(db *muxdb.MuxDB, proc func(state *state.State) error) *block.Block {
	blk, _, _, _ := new(genesis.Builder).
		Timestamp(uint64(time.Now().Unix())).
		State(proc).
		Build(state.NewStater(db))
	return blk
}

func inspectClauseWithBlockRef(clause *tx.Clause, blockRef *tx.BlockRef) ([]byte, uint64, error) {
	builder := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(42000).
		Clause(clause)

	if blockRef != nil {
		builder.BlockRef(*blockRef)
	}

	trx := builder.Build()
	return thorChain.ClauseCall(genesis.DevAccounts()[0], trx, 0)
}

func getClause(abi *abi.ABI, methodName string, address thor.Address, args ...any) (*tx.Clause, *abi.Method, error) {
	m, ok := abi.MethodByName(methodName)
	if !ok {
		return nil, nil, fmt.Errorf("method %s not found", methodName)
	}
	input, err := m.EncodeInput(args...)
	return tx.NewClause(&address).WithData(input), m, err
}

func callContractAndGetOutput(abi *abi.ABI, methodName string, address thor.Address, output any, args ...any) (uint64, error) {
	clause, m, err := getClause(abi, methodName, address, args...)
	if err != nil {
		return 0, err
	}
	decoded, gasUsed, err := inspectClauseWithBlockRef(clause, nil)
	if err != nil {
		return 0, err
	}
	return gasUsed, m.DecodeOutput(decoded, output)
}

func executeTxAndGetReceipt(description TestTxDescription) (*tx.Receipt, *thor.Bytes32, error) {
	m, ok := description.abi.MethodByName(description.methodName)
	if !ok {
		return nil, nil, fmt.Errorf("method %s not found", description.methodName)
	}
	input, err := m.EncodeInput(description.args...)
	if err != nil {
		return nil, nil, err
	}

	clause := tx.NewClause(&description.address).WithData(input)
	if description.vet != nil {
		clause = clause.WithValue(description.vet)
	}

	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Build()

	if description.duplicate {
		trx = new(tx.Builder).
			ChainTag(thorChain.Repo().ChainTag()).
			Expiration(50).
			Gas(100000).
			Clause(clause).
			Nonce(2).
			Build()
	}

	trx = tx.MustSign(trx, description.acc.PrivateKey)
	err = thorChain.MintTransactions(description.acc, trx)
	if err != nil {
		return nil, nil, err
	}

	id := trx.ID()
	fetchedTx, err := thorChain.GetTxReceipt(id)

	return fetchedTx, &id, err
}

func TestParamsNative(t *testing.T) {
	thorChain, _ = testchain.NewDefault()

	toAddr := builtin.Params.Address
	abi := builtin.Params.ABI

	var addr common.Address
	_, err := callContractAndGetOutput(abi, "executor", toAddr, &addr)

	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	key := thor.BytesToBytes32([]byte("key"))
	value := big.NewInt(999)
	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "set",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{key, value},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Params.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, key, fetchedTx.Outputs[0].Events[0].Topics[1])

	require.NoError(t, err)
	require.Equal(t, value, big.NewInt(0).SetBytes(fetchedTx.Outputs[0].Events[0].Data))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "set",
		address:    toAddr,
		acc:        genesis.DevAccounts()[1],
		args:       []any{key, value},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.True(t, fetchedTx.Reverted)

	var decodedVal *big.Int
	_, err = callContractAndGetOutput(abi, "get", toAddr, &decodedVal, key)
	require.NoError(t, err)
	require.Equal(t, value, decodedVal)
}

func TestAuthorityNative(t *testing.T) {
	thorChain, _ = testchain.NewDefault()
	var (
		master1   = genesis.DevAccounts()[1]
		endorsor1 = genesis.DevAccounts()[2]
		identity1 = genesis.DevAccounts()[3]

		master2   = genesis.DevAccounts()[4]
		endorsor2 = genesis.DevAccounts()[5]
		identity2 = genesis.DevAccounts()[6]

		master3   = genesis.DevAccounts()[7]
		endorsor3 = genesis.DevAccounts()[8]
		identity3 = genesis.DevAccounts()[9]
	)
	toAddr := builtin.Authority.Address

	abi := builtin.Authority.ABI

	var addr common.Address
	_, err := callContractAndGetOutput(abi, "first", toAddr, &addr)

	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	var b32 [32]uint8
	copy(b32[12:], identity1.Address.Bytes())
	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master1.Address, endorsor1.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	added := "added"
	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	copy(b32[12:], identity2.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master2.Address, endorsor2.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	copy(b32[12:], identity3.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master3.Address, endorsor3.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master3.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	_, err = callContractAndGetOutput(abi, "first", toAddr, &addr)
	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, genesis.DevAccounts()[0].Address)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, master1.Address)
	require.NoError(t, err)
	require.Equal(t, master2.Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, master2.Address)
	require.NoError(t, err)
	require.Equal(t, master3.Address.Bytes(), addr.Bytes())

	_, err = callContractAndGetOutput(abi, "next", toAddr, &addr, master3.Address)
	require.NoError(t, err)
	require.Equal(t, thor.Address{}.Bytes(), addr.Bytes())

	copy(b32[12:], identity1.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        master2,
		args:       []any{master1.Address, endorsor1.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        master1,
		args:       []any{master1.Address, endorsor1.Address, b32},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "revoke",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master1.Address},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	revoked := "revoked"
	require.NoError(t, err)
	require.Equal(t, revoked, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "revoke",
		address:    toAddr,
		acc:        genesis.DevAccounts()[0],
		args:       []any{master1.Address},
		duplicate:  true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	f := new(big.Float).SetFloat64(9.976e26)
	i := new(big.Int)
	f.Int(i)

	clause := tx.NewClause(&endorsor3.Address).WithValue(i)
	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(100000).
		Clause(clause).
		Nonce(2).
		Build()

	trx = tx.MustSign(trx, endorsor2.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(endorsor2, trx))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "revoke",
		address:    toAddr,
		acc:        master3,
		args:       []any{master2.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, revoked, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))
}

func TestEnergyNative(t *testing.T) {
	var (
		acc1 = genesis.DevAccounts()[0]
		acc2 = genesis.DevAccounts()[1]
		acc3 = thor.BytesToAddress([]byte("some acc"))
		acc4 = thor.BytesToAddress([]byte("stargate address"))
	)

	toAddr := builtin.Energy.Address

	abi := builtin.Energy.ABI

	fc := thor.SoloFork
	fc.HAYABUSA = 4
	thorChain, _ = testchain.NewWithFork(fc)

	var stringOutput string
	_, err := callContractAndGetOutput(abi, "name", toAddr, &stringOutput)

	veThor := "VeThor"
	require.NoError(t, err)
	require.Equal(t, veThor, stringOutput)

	var uint8Output uint8
	_, err = callContractAndGetOutput(abi, "decimals", toAddr, &uint8Output)

	exDecimals := uint8(18)
	require.NoError(t, err)
	require.Equal(t, exDecimals, uint8Output)

	_, err = callContractAndGetOutput(abi, "symbol", toAddr, &stringOutput)

	exSymbol := "VTHO"
	require.NoError(t, err)
	require.Equal(t, exSymbol, stringOutput)

	var bigIntOutput *big.Int
	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)

	exSupply := new(big.Int)
	exSupply.SetString("10000000000000000000000000000", 10)
	require.NoError(t, err)
	require.Equal(t, exSupply, bigIntOutput)

	_, err = callContractAndGetOutput(abi, "totalBurned", toAddr, &bigIntOutput)

	exBurned := big.Int{}
	require.NoError(t, err)
	require.Equal(t, exBurned.String(), bigIntOutput.String())

	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transfer",
		address:    toAddr,
		acc:        acc1,
		args:       []any{acc3, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc3, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	f := new(big.Float).SetFloat64(1e30)
	i := new(big.Int)
	f.Int(i)
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transfer",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc3, i},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "move",
		address:    toAddr,
		acc:        acc1,
		args:       []any{acc1.Address, acc3, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc3, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "move",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc1.Address, acc3, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "approve",
		address:    toAddr,
		acc:        acc1,
		args:       []any{acc2.Address, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	_, err = callContractAndGetOutput(abi, "allowance", toAddr, &bigIntOutput, acc1.Address, acc2.Address)

	exAllowance := big.NewInt(1001)
	require.NoError(t, err)
	require.Equal(t, exAllowance, bigIntOutput)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transferFrom",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc1.Address, acc3, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc3, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transferFrom",
		address:    toAddr,
		acc:        acc2,
		args:       []any{acc1.Address, acc3, big.NewInt(1000)},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)
	require.NoError(t, err)
	best := thorChain.Repo().BestBlockSummary().Header.Number()

	growth := new(big.Int)
	growth.SetUint64(thor.BlockInterval)
	growth.Mul(growth, exSupply)
	growth.Mul(growth, thor.EnergyGrowthRate)
	growth.Div(growth, big.NewInt(1e18))
	for i := uint32(0); i < best; i++ {
		exSupply = exSupply.Add(exSupply, growth)
	}
	require.Equal(t, exSupply, bigIntOutput)

	//_________________________________________________________
	// -- START SETUP HAYABUSA FORK AND TRANSITION TO POS --
	//---------------------------------------------------------

	//1: Set MaxBlockProposers to 1
	params := thorChain.Contract(builtin.Params.Address, builtin.Params.ABI, genesis.DevAccounts()[0])
	staker := thorChain.Contract(builtin.Staker.Address, builtin.Staker.ABI, genesis.DevAccounts()[0])
	assert.NoError(t, params.MintTransaction("set", big.NewInt(0), thor.KeyMaxBlockProposers, big.NewInt(1)))
	exSupply = exSupply.Add(exSupply, growth)
	assert.NoError(t, params.MintTransaction("set", big.NewInt(0), thor.KeyStargateContractAddress, big.NewInt(0).SetBytes(acc4.Bytes())))
	exSupply = exSupply.Add(exSupply, growth)

	// 2: Add a validator to the queue
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	assert.NoError(t, staker.MintTransaction("addValidator", minStake, acc1.Address, uint32(360)*24*7, true))
	exSupply = exSupply.Add(exSupply, growth)
	exSupply = exSupply.Add(exSupply, growth)

	validatorMap := make(map[uint64]*big.Int)
	stargateMap := make(map[uint64]*big.Int)

	// 3: Mint some blocks
	summary := thorChain.Repo().BestBlockSummary()
	firstPOS := summary.Header.Number() + 2
	st := thorChain.Stater().NewState(summary.Root())
	energyAtBlock, err := st.GetEnergy(summary.Header.Beneficiary(), summary.Header.Timestamp())
	require.NoError(t, err)
	validatorMap[summary.Header.Timestamp()] = energyAtBlock
	stargateBalAtBlock, err := st.GetEnergy(acc4, summary.Header.Timestamp())
	require.NoError(t, err)
	stargateMap[summary.Header.Timestamp()] = stargateBalAtBlock

	var totalSupplyBefore *big.Int
	var totalSupplyAfter *big.Int

	for range 5 {
		_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &totalSupplyBefore)
		require.NoError(t, err)
		require.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))
		summary = thorChain.Repo().BestBlockSummary()
		st := thorChain.Stater().NewState(summary.Root())
		energyAtBlock, err = st.GetEnergy(summary.Header.Beneficiary(), summary.Header.Timestamp())
		require.NoError(t, err)
		validatorMap[thorChain.Repo().BestBlockSummary().Header.Timestamp()] = energyAtBlock
		stargateBalAtBlock, err = st.GetEnergy(acc4, summary.Header.Timestamp())
		require.NoError(t, err)
		stargateMap[summary.Header.Timestamp()] = stargateBalAtBlock

		_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &totalSupplyAfter)
		require.NoError(t, err)

		validatorEnergyBefore := validatorMap[thorChain.Repo().BestBlockSummary().Header.Timestamp()-10]
		stargateEnergyBefore := stargateMap[thorChain.Repo().BestBlockSummary().Header.Timestamp()-10]

		if firstPOS <= summary.Header.Number() {
			// check after POS that sum of all rewards for block is equal to total supply growth
			require.Equal(t, big.NewInt(0).Add(big.NewInt(0).Sub(energyAtBlock, validatorEnergyBefore), big.NewInt(0).Sub(stargateBalAtBlock, stargateEnergyBefore)), big.NewInt(0).Sub(totalSupplyAfter, totalSupplyBefore))
		}
	}
	best = thorChain.Repo().BestBlockSummary().Header.Number()

	stakeRewards := big.NewInt(0)
	for i := firstPOS; i <= best; i++ {
		summary, err := thorChain.Repo().NewBestChain().GetBlockSummary(i)
		require.NoError(t, err)
		st = thorChain.Stater().NewState(summary.Root())
		staker := builtin.Staker.Native(st)
		energy := builtin.Energy.Native(st, summary.Header.Timestamp())
		reward, err := energy.CalculateRewards(staker)
		require.NoError(t, err)
		stakeRewards.Add(stakeRewards, reward)

		energyAtBlock := validatorMap[summary.Header.Timestamp()]
		energyBeforeBlock := validatorMap[summary.Header.Timestamp()-10]

		expectedReward := big.NewInt(0).Mul(reward, big.NewInt(3))
		expectedReward = expectedReward.Div(expectedReward, big.NewInt(10))
		require.Equal(t, expectedReward, big.NewInt(0).Sub(energyAtBlock, energyBeforeBlock))

		stargateEnergyAtBlock := stargateMap[summary.Header.Timestamp()]
		stargateEnergyBeforeBlock := stargateMap[summary.Header.Timestamp()-10]
		require.Equal(t, big.NewInt(0).Sub(reward, expectedReward), big.NewInt(0).Sub(stargateEnergyAtBlock, stargateEnergyBeforeBlock))
	}
	exSupply = exSupply.Add(exSupply, stakeRewards)

	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)
	require.NoError(t, err)
	require.Equal(t, exSupply, bigIntOutput)
}

func TestPrototypeNative(t *testing.T) {
	var (
		acc1         = thor.BytesToAddress([]byte("acc1"))
		master1      = genesis.DevAccounts()[0]
		master2      = genesis.DevAccounts()[1]
		sponsor      = genesis.DevAccounts()[2]
		credit       = big.NewInt(1000)
		recoveryRate = big.NewInt(10)
	)

	thorChain, _ = testchain.NewDefault()
	abi := builtin.Prototype.ABI
	toAddr := builtin.Prototype.Address

	code, _ := hex.DecodeString("60606040523415600e57600080fd5b603580601b6000396000f3006060604052600080fd00a165627a7a72305820edd8a93b651b5aac38098767f0537d9b25433278c9d155da2135efc06927fc960029")
	clause := tx.NewClause(nil).WithData(code)
	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(100000).
		Clause(clause).
		Build()

	trx = tx.MustSign(trx, master1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(master1, trx))

	id := trx.ID()
	fetchedTx, err := thorChain.GetTxReceipt(id)
	assert.NoError(t, err)
	contractAddr := fetchedTx.Outputs[0].Events[0].Address

	var outputAddr common.Address
	_, err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, acc1)

	exMaster := thor.Address{}
	require.NoError(t, err)
	require.Equal(t, exMaster.Bytes(), outputAddr.Bytes())

	_, err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setMaster",
		address:    toAddr,
		acc:        master2,
		args:       []any{master2.Address, master1.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, master2.Address, fetchedTx.Outputs[0].Events[0].Address)

	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setMaster",
		address:    toAddr,
		acc:        master2,
		args:       []any{master1.Address, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, master2.Address)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), outputAddr.Bytes())

	var outputBool bool
	_, err = callContractAndGetOutput(abi, "hasCode", toAddr, &outputBool, master1.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	_, err = callContractAndGetOutput(abi, "hasCode", toAddr, &outputBool, contractAddr)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setCreditPlan",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, credit, recoveryRate},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)

	decodedVal := fetchedTx.Outputs[0].Events[0].Data

	require.NoError(t, err)
	require.Equal(t, credit.Bytes(), bytes.Trim(decodedVal[:32], "\x00"))
	require.Equal(t, recoveryRate.Bytes(), bytes.Trim(decodedVal[32:], "\x00"))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setCreditPlan",
		address:    toAddr,
		acc:        master2,
		args:       []any{contractAddr, credit, recoveryRate},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	type CreditPlan struct {
		Credit       *big.Int
		RecoveryRate *big.Int
	}
	plan := &CreditPlan{}
	_, err = callContractAndGetOutput(abi, "creditPlan", toAddr, plan, contractAddr)

	require.NoError(t, err)
	require.Equal(t, credit.String(), plan.Credit.String())
	require.Equal(t, recoveryRate, plan.RecoveryRate)

	_, err = callContractAndGetOutput(abi, "isUser", toAddr, &outputBool, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	added := "added"
	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master2,
		args:       []any{contractAddr, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, acc1},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "isUser", toAddr, &outputBool, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	var outputBigInt *big.Int
	_, err = callContractAndGetOutput(abi, "userCredit", toAddr, &outputBigInt, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, credit, outputBigInt)

	_, err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "sponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, sponsor.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	sponsored := "sponsored"
	require.NoError(t, err)
	require.Equal(t, sponsored, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "sponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	_, err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, common.Address{}, outputAddr)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, sponsor.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, contractAddr, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, sponsor.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	selected := "selected"
	require.NoError(t, err)
	require.Equal(t, selected, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master1,
		args:       []any{contractAddr, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master2,
		args:       []any{contractAddr, sponsor.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, sponsor.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "unsponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, sponsor.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "unsponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []any{contractAddr},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	_, err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	storageKey := thor.Blake2b(contractAddr.Bytes(), []byte("credit-plan"))
	ch := thorChain.Repo().NewBestChain()
	summary, err := thorChain.Repo().GetBlockSummary(ch.HeadID())
	assert.NoError(t, err)
	st := state.New(thorChain.Database(), trie.Root{Hash: summary.Header.StateRoot(), Ver: trie.Version{Major: summary.Header.Number()}})
	storageValDecoded, err := st.GetStorage(toAddr, storageKey)
	assert.NoError(t, err)

	var uint8Array [32]uint8
	_, err = callContractAndGetOutput(abi, "storageFor", toAddr, &uint8Array, builtin.Prototype.Address, thor.Blake2b([]byte("credit-plan")))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])

	_, err = callContractAndGetOutput(abi, "storageFor", toAddr, &uint8Array, builtin.Prototype.Address, thor.Blake2b(contractAddr.Bytes(), []byte("credit-plan")))

	require.NoError(t, err)
	require.Equal(t, storageValDecoded.Bytes(), uint8Array[:])

	var outputInt *big.Int
	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputInt, acc1, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputInt.String())

	clause = tx.NewClause(&acc1).WithValue(big.NewInt(1001))
	trx = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Build()

	trx = tx.MustSign(trx, master1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(master1, trx))
	bestBlock, err := thorChain.BestBlock()
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputInt, acc1, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001), outputInt)

	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputInt, acc1, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputInt.String())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []any{acc1, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputInt, acc1, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000), outputInt)
}

func TestPrototypeNativeWithLongerBlockNumber(t *testing.T) {
	var (
		acc1   = genesis.DevAccounts()[1]
		acc2   = thor.BytesToAddress([]byte("acc2"))
		toAddr = builtin.Prototype.Address
		abi    = builtin.Prototype.ABI
	)
	thorChain, _ = testchain.NewDefault()

	var outputBigInt *big.Int
	_, err := callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputBigInt.String())

	clause := tx.NewClause(&acc2).WithValue(big.NewInt(1))
	trx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Build()

	trx = tx.MustSign(trx, acc1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(acc1, trx))
	bestBlock, err := thorChain.BestBlock()
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), outputBigInt)

	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputBigInt.String())

	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        acc1,
		args:       []any{acc2, big.NewInt(1)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), outputBigInt)

	clause = tx.NewClause(&acc2).WithValue(big.NewInt(1))
	trx = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(100000).
		Clause(clause).
		Nonce(2).
		Build()

	trx = tx.MustSign(trx, acc1.PrivateKey)
	require.NoError(t, thorChain.MintTransactions(acc1, trx))
	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)

	_, err = callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), outputBigInt)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        acc1,
		args:       []any{acc2, big.NewInt(1)},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), outputBigInt)
}

func TestExtensionNative(t *testing.T) {
	thorChain, _ = testchain.NewDefault()

	master1 := genesis.DevAccounts()[0]
	master2 := genesis.DevAccounts()[1]
	b0 := thorChain.GenesisBlock()
	abi := builtin.Extension.V2.ABI
	toAddr := builtin.Extension.Address

	var uint8Array [32]uint8
	_, err := callContractAndGetOutput(abi, "blake2b256", toAddr, &uint8Array, []byte("hello world"))

	require.NoError(t, err)
	require.Equal(t, thor.Blake2b([]byte("hello world")).Bytes(), uint8Array[:])

	var expected *big.Int
	var bigIntOutput *big.Int
	_, err = callContractAndGetOutput(builtin.Energy.ABI, "totalSupply", builtin.Energy.Address, &expected)
	assert.NoError(t, err)
	_, err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)

	require.NoError(t, err)
	require.Equal(t, expected, bigIntOutput)

	m, _ := abi.MethodByName("txBlockRef")
	input, err := m.EncodeInput()
	assert.NoError(t, err)
	clause := tx.NewClause(&toAddr).WithData(input)
	blkRef := tx.NewBlockRef(1)

	outBytes, _, err := inspectClauseWithBlockRef(clause, &blkRef)

	require.NoError(t, err)
	require.Equal(t, tx.NewBlockRef(1).Number(), binary.BigEndian.Uint32(outBytes))

	_, err = callContractAndGetOutput(abi, "txExpiration", toAddr, &bigIntOutput)

	require.NoError(t, err)
	require.Equal(t, big.NewInt(50), bigIntOutput)

	m, _ = abi.MethodByName("txProvedWork")
	input, err = m.EncodeInput()
	assert.NoError(t, err)
	clause = tx.NewClause(&toAddr).WithData(input)

	builder := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(42000).
		Clause(clause)

	trx := builder.Build()
	outBytes, _, err = thorChain.ClauseCall(master1, trx, 0)
	assert.NoError(t, err)

	block, err := thorChain.BestBlock()
	assert.NoError(t, err)

	getBlockID := func(_ uint32) (thor.Bytes32, error) {
		return thor.Bytes32{}, nil
	}
	provedWork, err := trx.ProvedWork(block.Header().Number(), getBlockID)

	require.NoError(t, err)
	require.Equal(t, provedWork.String(), new(big.Int).SetBytes(outBytes).String())

	m, _ = abi.MethodByName("txID")
	input, err = m.EncodeInput()
	assert.NoError(t, err)
	clause = tx.NewClause(&toAddr).WithData(input)

	builder = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(50).
		Gas(42000).
		Clause(clause)

	trx = builder.Build()
	trxID, _, err := thorChain.ClauseCall(master1, trx, 0)
	assert.NoError(t, err)

	require.NoError(t, err)
	require.Equal(t, trx.ID().Bytes(), trxID)

	fetchedTx, id, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []any{master2.Address, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	fetchedTx2, id2, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []any{master2.Address, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx2.Outputs))
	require.False(t, fetchedTx2.Reverted)

	gasUsed, err := callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(3))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])
	assert.Equal(t, uint64(570), gasUsed)

	gasUsed, err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(2))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])
	assert.Equal(t, uint64(570), gasUsed)

	gasUsed, err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(1))

	require.NoError(t, err)
	assert.Equal(t, uint64(770), gasUsed)
	bl, err := thorChain.GetTxBlock(id)
	require.NoError(t, err)
	require.Equal(t, bl.Header().ID().Bytes(), uint8Array[:])

	gasUsed, err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(0))
	require.NoError(t, err)
	assert.Equal(t, uint64(770), gasUsed)
	require.Equal(t, b0.Header().ID().Bytes(), uint8Array[:])

	var uint64Output uint64
	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(3))
	require.NoError(t, err)
	assert.Equal(t, uint64(454), gasUsed)
	require.Equal(t, uint64(0), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(2))
	assert.NoError(t, err)
	assert.Equal(t, uint64(454), gasUsed)
	bl2, err := thorChain.GetTxBlock(id2)
	require.NoError(t, err)
	block2, err := thorChain.Repo().GetBlock(bl2.Header().ID())

	require.NoError(t, err)
	require.Equal(t, block2.Header().TotalScore(), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(1))
	assert.Equal(t, uint64(854), gasUsed)
	assert.NoError(t, err)
	block1, err := thorChain.Repo().GetBlock(bl.Header().ID())

	require.NoError(t, err)
	require.Equal(t, block1.Header().TotalScore(), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(0))

	require.NoError(t, err)
	assert.Equal(t, uint64(854), gasUsed)
	require.Equal(t, b0.Header().TotalScore(), uint64Output)

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(3))

	require.NoError(t, err)
	assert.Equal(t, uint64(404), gasUsed)
	require.Equal(t, big.NewInt(0).String(), bigIntOutput.String())

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(2))

	require.NoError(t, err)
	assert.Equal(t, uint64(404), gasUsed)
	require.Equal(t, new(big.Int).SetUint64(block2.Header().Timestamp()), bigIntOutput)

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(1))

	require.NoError(t, err)
	assert.Equal(t, uint64(804), gasUsed)
	require.Equal(t, new(big.Int).SetUint64(block1.Header().Timestamp()), bigIntOutput)

	gasUsed, err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(0))

	require.NoError(t, err)
	assert.Equal(t, uint64(804), gasUsed)
	require.Equal(t, new(big.Int).SetUint64(b0.Header().Timestamp()), bigIntOutput)

	var addressOutput common.Address
	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(3))

	require.NoError(t, err)
	assert.Equal(t, uint64(432), gasUsed)
	require.Equal(t, common.Address{}, addressOutput)

	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(2))

	require.NoError(t, err)
	assert.Equal(t, uint64(432), gasUsed)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())

	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(1))

	require.NoError(t, err)
	assert.Equal(t, uint64(832), gasUsed)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())

	gasUsed, err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(0))

	require.NoError(t, err)
	assert.Equal(t, uint64(832), gasUsed)
	require.Equal(t, common.Address{}, addressOutput)

	gasUsed, err = callContractAndGetOutput(abi, "txGasPayer", toAddr, &addressOutput)

	require.NoError(t, err)
	assert.Equal(t, uint64(372), gasUsed)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())
}

func TestStakerContract_Native(t *testing.T) {
	fc := thor.SoloFork
	fc.HAYABUSA = 2
	fc.HAYABUSA_TP = 2
	var err error
	thorChain, err = testchain.NewWithFork(fc)
	assert.NoError(t, err)

	energyAbi := builtin.Energy.ABI
	energyAddress := builtin.Energy.Address

	mbp := TestTxDescription{
		t:          t,
		abi:        builtin.Params.ABI,
		methodName: "set",
		address:    builtin.Params.Address,
		acc:        genesis.DevAccounts()[0],
		args:       []any{thor.KeyMaxBlockProposers, big.NewInt(1)},
	}
	receipt, a, err := executeTxAndGetReceipt(mbp) // mint block 1
	assert.NoError(t, err)
	assert.NotNil(t, receipt)
	assert.NotNil(t, a)

	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 2: hayabusa should fork here and set the contract bytecode

	totalBurnedBefore := new(big.Int)
	_, err = callContractAndGetOutput(energyAbi, "totalBurned", energyAddress, &totalBurnedBefore)
	assert.NoError(t, err)

	abi := builtin.Staker.ABI
	toAddr := builtin.Staker.Address
	endorsor := genesis.DevAccounts()[9]
	master := genesis.DevAccounts()[8]

	// parameters
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	minStakingPeriod := uint32(360) * 24 * 14

	// addValidator
	addValidatorArgs := []any{master.Address, minStakingPeriod, true}
	desc := TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addValidator",
		address:    toAddr,
		acc:        endorsor,
		args:       addValidatorArgs,
		vet:        minStake,
	}
	receipt, _, err = executeTxAndGetReceipt(desc) // mint block 3
	assert.NoError(t, err)
	id := receipt.Outputs[0].Events[0].Topics[3]

	totalBurned := new(big.Int)
	_, err = callContractAndGetOutput(energyAbi, "totalBurned", energyAddress, &totalBurned)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Mul(big.NewInt(0).SetUint64(receipt.GasUsed), big.NewInt(1e15)), big.NewInt(0).Sub(totalBurned, totalBurnedBefore))

	// get
	getRes := make([]any, 6)
	getRes[0] = new(common.Address)
	getRes[1] = new(common.Address)
	getRes[2] = new(*big.Int)
	getRes[3] = new(*big.Int)
	getRes[4] = new(uint8)
	getRes[5] = new(bool)
	_, err = callContractAndGetOutput(abi, "get", toAddr, &getRes, id)
	assert.NoError(t, err)
	expectedEndorsor := common.BytesToAddress(endorsor.Address.Bytes())
	expectedMasterAddress := common.BytesToAddress(master.Address.Bytes())
	assert.Equal(t, &expectedMasterAddress, getRes[0])
	assert.Equal(t, &expectedEndorsor, getRes[1])
	assert.Equal(t, big.NewInt(0).Cmp(*getRes[2].(**big.Int)), 0) // stake - should be 0 while queued
	assert.Equal(t, big.NewInt(0).Cmp(*getRes[3].(**big.Int)), 0) // weight - should be 0 while queued
	assert.Equal(t, staker.StatusQueued, *getRes[4].(*uint8))
	assert.Equal(t, true, *getRes[5].(*bool)) // isMaster

	//firstQueued
	firstQueuedRes := new(common.Hash)
	_, err = callContractAndGetOutput(abi, "firstQueued", toAddr, firstQueuedRes)
	assert.NoError(t, err)
	expectedMaster := common.BytesToHash(id.Bytes())
	assert.Equal(t, &expectedMaster, firstQueuedRes)

	// queuedStake
	queuedStake := new(*big.Int)
	_, err = callContractAndGetOutput(abi, "queuedStake", toAddr, queuedStake)
	assert.NoError(t, err)
	expectedTotalStake := big.NewInt(25_000_000)
	expectedTotalStake = expectedTotalStake.Mul(expectedTotalStake, big.NewInt(1e18))
	assert.Equal(t, expectedTotalStake, *queuedStake)

	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 4: PoS should become active and active the queued validators

	// firstActive
	firstActiveRes := new(common.Hash)
	_, err = callContractAndGetOutput(abi, "firstActive", toAddr, firstActiveRes)
	assert.NoError(t, err)
	expectedFirst := common.BytesToHash(id.Bytes())
	assert.Equal(t, &expectedFirst, firstActiveRes)

	// totalStake
	totalStake := new(*big.Int)
	_, err = callContractAndGetOutput(abi, "totalStake", toAddr, totalStake)
	assert.NoError(t, err)
	expectedTotalStake = big.NewInt(25_000_000)
	expectedTotalStake = expectedTotalStake.Mul(expectedTotalStake, big.NewInt(1e18))
	assert.Equal(t, expectedTotalStake, *totalStake)

	// queuedStake
	queuedStake = new(*big.Int)
	_, err = callContractAndGetOutput(abi, "queuedStake", toAddr, queuedStake)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), (*queuedStake).String())
}

func TestStakerContract_Native_Revert(t *testing.T) {
	fc := thor.SoloFork
	fc.HAYABUSA = 2
	fc.HAYABUSA_TP = 2
	var err error
	thorChain, err = testchain.NewWithFork(fc)
	assert.NoError(t, err)

	mbp := TestTxDescription{
		t:          t,
		abi:        builtin.Params.ABI,
		methodName: "set",
		address:    builtin.Params.Address,
		acc:        genesis.DevAccounts()[0],
		args:       []any{thor.KeyMaxBlockProposers, big.NewInt(1)},
	}
	receipt, a, err := executeTxAndGetReceipt(mbp) // mint block 1
	assert.NoError(t, err)
	assert.NotNil(t, receipt)
	assert.NotNil(t, a)

	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0])) // mint block 2: hayabusa should fork here and set the contract bytecode

	abi := builtin.Staker.ABI
	toAddr := builtin.Staker.Address
	endorsor := genesis.DevAccounts()[9]
	master := genesis.DevAccounts()[8]

	// parameters
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	minStakingPeriod := uint32(360) * 24 * 14

	// addValidator
	addValidatorArgs := []any{master.Address, minStakingPeriod + 1, true}
	desc := TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addValidator",
		address:    toAddr,
		acc:        endorsor,
		args:       addValidatorArgs,
		vet:        minStake,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	//update auto renew
	updateAutoRenewArgs := []any{thor.Bytes32{}, false}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "updateAutoRenew",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       updateAutoRenewArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	//increase stake
	increaseStakeArgs := []any{thor.Bytes32{}}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "increaseStake",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       increaseStakeArgs,
		vet:        big.NewInt(1),
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	//decrease stake
	decreaseStakeArgs := []any{thor.Bytes32{}, big.NewInt(1)}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "decreaseStake",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       decreaseStakeArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// update delegator auto renew
	updateDelegatorAutoRenewArgs := []any{thor.Bytes32{}, thor.Address{}, false}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "updateDelegatorAutoRenew",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       updateDelegatorAutoRenewArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// addDelegation
	addDelegationArgs := []any{thor.Bytes32{}, thor.Address{}, false, uint8(1)}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addDelegation",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       addDelegationArgs,
		vet:        big.NewInt(10),
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)

	// withdrawDelegation
	withdrawDelegationArgs := []any{thor.Bytes32{}, thor.Address{}}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "withdrawDelegation",
		address:    toAddr,
		acc:        genesis.DevAccounts()[2],
		args:       withdrawDelegationArgs,
	}
	receipt, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted)
}

func TestStakerContract_Native_WithdrawQueued(t *testing.T) {
	fc := thor.SoloFork
	fc.HAYABUSA = 1
	fc.HAYABUSA_TP = 2
	var err error
	thorChain, err = testchain.NewWithFork(fc)
	assert.NoError(t, err)
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	abi := builtin.Staker.ABI
	toAddr := builtin.Staker.Address
	endorsor := genesis.DevAccounts()[9]
	master := genesis.DevAccounts()[8]

	// parameters
	minStake := big.NewInt(25_000_000)
	minStake = minStake.Mul(minStake, big.NewInt(1e18))
	minStakingPeriod := uint32(360) * 24 * 14

	// addValidator
	addValidatorArgs := []any{master.Address, minStakingPeriod, false}
	desc := TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addValidator",
		address:    toAddr,
		acc:        endorsor,
		args:       addValidatorArgs,
		vet:        minStake,
	}
	receipt, _, err := executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	id := receipt.Outputs[0].Events[0].Topics[3]

	// withdraw queued
	withdrawArgs := []any{id}
	desc = TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "withdraw",
		address:    toAddr,
		acc:        endorsor,
		args:       withdrawArgs,
		vet:        big.NewInt(0),
	}
	_, _, err = executeTxAndGetReceipt(desc)
	assert.NoError(t, err)
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	// get
	getRes := make([]any, 6)
	getRes[0] = new(common.Address)
	getRes[1] = new(common.Address)
	getRes[2] = new(*big.Int)
	getRes[3] = new(*big.Int)
	getRes[4] = new(uint8)
	getRes[5] = new(bool)
	_, err = callContractAndGetOutput(abi, "get", toAddr, &getRes, id)
	assert.NoError(t, err)
	assert.Equal(t, staker.StatusExit, *getRes[4].(*uint8))

	//firstQueued
	firstQueuedRes := new(common.Hash)
	_, err = callContractAndGetOutput(abi, "firstQueued", toAddr, firstQueuedRes)
	assert.NoError(t, err)
	expectedMaster := common.Hash{}
	assert.Equal(t, &expectedMaster, firstQueuedRes)
}
