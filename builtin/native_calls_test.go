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
	args       []interface{}
	events     tx.Events
	provedWork *big.Int
	txID       thor.Bytes32
	blockRef   tx.BlockRef
	gasPayer   thor.Address
	expiration uint32
	value      *big.Int

	output *[]interface{}
	vmerr  error
}

type TestTxDescription struct {
	t          *testing.T
	abi        *abi.ABI
	methodName string
	address    thor.Address
	acc        genesis.DevAccount
	args       []interface{}
	duplicate  bool
}

func (c *ctest) Case(name string, args ...interface{}) *ccase {
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

func (c *ccase) ShouldOutput(outputs ...interface{}) *ccase {
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

func inspectClauseWithBlockRef(clause *tx.Clause, blockRef *tx.BlockRef) ([]byte, error) {
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

func getClause(abi *abi.ABI, methodName string, address thor.Address, args ...interface{}) (*tx.Clause, *abi.Method, error) {
	m, ok := abi.MethodByName(methodName)
	if !ok {
		return nil, nil, fmt.Errorf("method %s not found", methodName)
	}
	input, err := m.EncodeInput(args...)
	return tx.NewClause(&address).WithData(input), m, err
}

func callContractAndGetOutput(abi *abi.ABI, methodName string, address thor.Address, output interface{}, args ...interface{}) error {
	clause, m, err := getClause(abi, methodName, address, args...)
	if err != nil {
		return err
	}
	decoded, err := inspectClauseWithBlockRef(clause, nil)
	if err != nil {
		return err
	}
	return m.DecodeOutput(decoded, output)
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
	thorChain, _ = testchain.NewIntegrationTestChain()

	toAddr := builtin.Params.Address
	abi := builtin.Params.ABI

	var addr common.Address
	err := callContractAndGetOutput(abi, "executor", toAddr, &addr)

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
		args:       []interface{}{key, value},
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
		args:       []interface{}{key, value},
		duplicate:  false,
	})
	require.NoError(t, err)
	require.True(t, fetchedTx.Reverted)

	var decodedVal *big.Int
	err = callContractAndGetOutput(abi, "get", toAddr, &decodedVal, key)
	require.NoError(t, err)
	require.Equal(t, value, decodedVal)
}

func TestAuthorityNative(t *testing.T) {
	thorChain, _ = testchain.NewIntegrationTestChain()
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
	err := callContractAndGetOutput(abi, "first", toAddr, &addr)

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
		args:       []interface{}{master1.Address, endorsor1.Address, b32},
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
		args:       []interface{}{master2.Address, endorsor2.Address, b32},
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
		args:       []interface{}{master3.Address, endorsor3.Address, b32},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Authority.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, master3.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))

	require.NoError(t, err)
	require.Equal(t, added, string(bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00")))

	err = callContractAndGetOutput(abi, "first", toAddr, &addr)
	require.NoError(t, err)
	require.Equal(t, genesis.DevAccounts()[0].Address.Bytes(), addr.Bytes())

	err = callContractAndGetOutput(abi, "next", toAddr, &addr, genesis.DevAccounts()[0].Address)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), addr.Bytes())

	err = callContractAndGetOutput(abi, "next", toAddr, &addr, master1.Address)
	require.NoError(t, err)
	require.Equal(t, master2.Address.Bytes(), addr.Bytes())

	err = callContractAndGetOutput(abi, "next", toAddr, &addr, master2.Address)
	require.NoError(t, err)
	require.Equal(t, master3.Address.Bytes(), addr.Bytes())

	err = callContractAndGetOutput(abi, "next", toAddr, &addr, master3.Address)
	require.NoError(t, err)
	require.Equal(t, thor.Address{}.Bytes(), addr.Bytes())

	copy(b32[12:], identity1.Address.Bytes())
	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "add",
		address:    toAddr,
		acc:        master2,
		args:       []interface{}{master1.Address, endorsor1.Address, b32},
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
		args:       []interface{}{master1.Address, endorsor1.Address, b32},
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
		args:       []interface{}{master1.Address},
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
		args:       []interface{}{master1.Address},
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
		args:       []interface{}{master2.Address},
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
	)

	toAddr := builtin.Energy.Address

	abi := builtin.Energy.ABI
	thorChain, _ = testchain.NewIntegrationTestChain()

	var stringOutput string
	err := callContractAndGetOutput(abi, "name", toAddr, &stringOutput)

	veThor := "VeThor"
	require.NoError(t, err)
	require.Equal(t, veThor, stringOutput)

	var uint8Output uint8
	err = callContractAndGetOutput(abi, "decimals", toAddr, &uint8Output)

	exDecimals := uint8(18)
	require.NoError(t, err)
	require.Equal(t, exDecimals, uint8Output)

	err = callContractAndGetOutput(abi, "symbol", toAddr, &stringOutput)

	exSymbol := "VTHO"
	require.NoError(t, err)
	require.Equal(t, exSymbol, stringOutput)

	var bigIntOutput *big.Int
	err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)

	exSupply := new(big.Int)
	exSupply.SetString("10000000000000000000000000000", 10)
	require.NoError(t, err)
	require.Equal(t, exSupply, bigIntOutput)

	err = callContractAndGetOutput(abi, "totalBurned", toAddr, &bigIntOutput)

	exBurned := big.Int{}
	require.NoError(t, err)
	require.Equal(t, exBurned.String(), bigIntOutput.String())

	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transfer",
		address:    toAddr,
		acc:        acc1,
		args:       []interface{}{acc3, big.NewInt(1000)},
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
		args:       []interface{}{acc3, i},
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
		args:       []interface{}{acc1.Address, acc3, big.NewInt(1001)},
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
		args:       []interface{}{acc1.Address, acc3, big.NewInt(1001)},
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
		args:       []interface{}{acc2.Address, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.Equal(t, builtin.Energy.Address, fetchedTx.Outputs[0].Events[0].Address)
	require.Equal(t, acc1.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[1].Bytes()))
	require.Equal(t, acc2.Address, thor.BytesToAddress(fetchedTx.Outputs[0].Events[0].Topics[2].Bytes()))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001).Bytes(), bytes.Trim(fetchedTx.Outputs[0].Events[0].Data, "\x00"))

	err = callContractAndGetOutput(abi, "allowance", toAddr, &bigIntOutput, acc1.Address, acc2.Address)

	exAllowance := big.NewInt(1001)
	require.NoError(t, err)
	require.Equal(t, exAllowance, bigIntOutput)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "transferFrom",
		address:    toAddr,
		acc:        acc2,
		args:       []interface{}{acc1.Address, acc3, big.NewInt(1000)},
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
		args:       []interface{}{acc1.Address, acc3, big.NewInt(1000)},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)
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

	thorChain, _ = testchain.NewIntegrationTestChain()
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
	err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, acc1)

	exMaster := thor.Address{}
	require.NoError(t, err)
	require.Equal(t, exMaster.Bytes(), outputAddr.Bytes())

	err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setMaster",
		address:    toAddr,
		acc:        master2,
		args:       []interface{}{master2.Address, master1.Address},
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
		args:       []interface{}{master1.Address, acc1},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	err = callContractAndGetOutput(abi, "master", toAddr, &outputAddr, master2.Address)
	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), outputAddr.Bytes())

	var outputBool bool
	err = callContractAndGetOutput(abi, "hasCode", toAddr, &outputBool, master1.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	err = callContractAndGetOutput(abi, "hasCode", toAddr, &outputBool, contractAddr)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "setCreditPlan",
		address:    toAddr,
		acc:        master1,
		args:       []interface{}{contractAddr, credit, recoveryRate},
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
		args:       []interface{}{contractAddr, credit, recoveryRate},
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
	err = callContractAndGetOutput(abi, "creditPlan", toAddr, plan, contractAddr)

	require.NoError(t, err)
	require.Equal(t, credit.String(), plan.Credit.String())
	require.Equal(t, recoveryRate, plan.RecoveryRate)

	err = callContractAndGetOutput(abi, "isUser", toAddr, &outputBool, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "addUser",
		address:    toAddr,
		acc:        master1,
		args:       []interface{}{contractAddr, acc1},
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
		args:       []interface{}{contractAddr, acc1},
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
		args:       []interface{}{contractAddr, acc1},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	err = callContractAndGetOutput(abi, "isUser", toAddr, &outputBool, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	var outputBigInt *big.Int
	err = callContractAndGetOutput(abi, "userCredit", toAddr, &outputBigInt, contractAddr, acc1)
	require.NoError(t, err)
	require.Equal(t, credit, outputBigInt)

	err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, false, outputBool)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "sponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []interface{}{contractAddr},
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
		args:       []interface{}{contractAddr},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
	require.NoError(t, err)
	require.Equal(t, true, outputBool)

	err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, common.Address{}, outputAddr)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "selectSponsor",
		address:    toAddr,
		acc:        master1,
		args:       []interface{}{contractAddr, sponsor.Address},
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
		args:       []interface{}{contractAddr, acc1},
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
		args:       []interface{}{contractAddr, sponsor.Address},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, sponsor.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "unsponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []interface{}{contractAddr},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	err = callContractAndGetOutput(abi, "currentSponsor", toAddr, &outputAddr, contractAddr)
	require.NoError(t, err)
	require.Equal(t, sponsor.Address.Bytes(), outputAddr.Bytes())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        abi,
		methodName: "unsponsor",
		address:    toAddr,
		acc:        sponsor,
		args:       []interface{}{contractAddr},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(fetchedTx.Outputs))
	require.True(t, fetchedTx.Reverted)

	err = callContractAndGetOutput(abi, "isSponsor", toAddr, &outputBool, contractAddr, sponsor.Address)
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
	err = callContractAndGetOutput(abi, "storageFor", toAddr, &uint8Array, builtin.Prototype.Address, thor.Blake2b([]byte("credit-plan")))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])

	err = callContractAndGetOutput(abi, "storageFor", toAddr, &uint8Array, builtin.Prototype.Address, thor.Blake2b(contractAddr.Bytes(), []byte("credit-plan")))

	require.NoError(t, err)
	require.Equal(t, storageValDecoded.Bytes(), uint8Array[:])

	var outputInt *big.Int
	err = callContractAndGetOutput(abi, "balance", toAddr, &outputInt, acc1, big.NewInt(0))

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

	err = callContractAndGetOutput(abi, "balance", toAddr, &outputInt, acc1, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1001), outputInt)

	err = callContractAndGetOutput(abi, "energy", toAddr, &outputInt, acc1, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputInt.String())

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []interface{}{acc1, big.NewInt(1000)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	err = callContractAndGetOutput(abi, "energy", toAddr, &outputInt, acc1, big.NewInt(int64(bestBlock.Header().Number())))

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
	thorChain, _ = testchain.NewIntegrationTestChain()

	var outputBigInt *big.Int
	err := callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(0))

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

	err = callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), outputBigInt)

	err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), outputBigInt.String())

	fetchedTx, _, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        acc1,
		args:       []interface{}{acc2, big.NewInt(1)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

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

	err = callContractAndGetOutput(abi, "balance", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), outputBigInt)

	fetchedTx, _, err = executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        acc1,
		args:       []interface{}{acc2, big.NewInt(1)},
		duplicate:  true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx.Outputs))
	require.False(t, fetchedTx.Reverted)

	bestBlock, err = thorChain.BestBlock()
	assert.NoError(t, err)
	err = callContractAndGetOutput(abi, "energy", toAddr, &outputBigInt, acc2, big.NewInt(int64(bestBlock.Header().Number())))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), outputBigInt)
}

func TestExtensionNative(t *testing.T) {
	thorChain, _ = testchain.NewIntegrationTestChain()

	master1 := genesis.DevAccounts()[0]
	master2 := genesis.DevAccounts()[1]
	b0 := thorChain.GenesisBlock()
	abi := builtin.Extension.V2.ABI
	toAddr := builtin.Extension.Address

	var uint8Array [32]uint8
	err := callContractAndGetOutput(abi, "blake2b256", toAddr, &uint8Array, []byte("hello world"))

	require.NoError(t, err)
	require.Equal(t, thor.Blake2b([]byte("hello world")).Bytes(), uint8Array[:])

	var expected *big.Int
	var bigIntOutput *big.Int
	err = callContractAndGetOutput(builtin.Energy.ABI, "totalSupply", builtin.Energy.Address, &expected)
	assert.NoError(t, err)
	err = callContractAndGetOutput(abi, "totalSupply", toAddr, &bigIntOutput)

	require.NoError(t, err)
	require.Equal(t, expected, bigIntOutput)

	m, _ := abi.MethodByName("txBlockRef")
	input, err := m.EncodeInput()
	assert.NoError(t, err)
	clause := tx.NewClause(&toAddr).WithData(input)
	blkRef := tx.NewBlockRef(1)

	outBytes, err := inspectClauseWithBlockRef(clause, &blkRef)

	require.NoError(t, err)
	require.Equal(t, tx.NewBlockRef(1).Number(), binary.BigEndian.Uint32(outBytes))

	err = callContractAndGetOutput(abi, "txExpiration", toAddr, &bigIntOutput)

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
	outBytes, err = thorChain.ClauseCall(master1, trx, 0)
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
	trxID, err := thorChain.ClauseCall(master1, trx, 0)
	assert.NoError(t, err)

	require.NoError(t, err)
	require.Equal(t, trx.ID().Bytes(), trxID)

	fetchedTx, id, err := executeTxAndGetReceipt(TestTxDescription{
		t:          t,
		abi:        builtin.Energy.ABI,
		methodName: "transfer",
		address:    builtin.Energy.Address,
		acc:        master1,
		args:       []interface{}{master2.Address, big.NewInt(1000)},
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
		args:       []interface{}{master2.Address, big.NewInt(1001)},
		duplicate:  false,
	})

	require.NoError(t, err)
	require.Equal(t, 1, len(fetchedTx2.Outputs))
	require.False(t, fetchedTx2.Reverted)

	err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(3))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])

	err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(2))

	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}.Bytes(), uint8Array[:])

	err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(1))
	require.NoError(t, err)

	bl, err := thorChain.GetTxBlock(id)
	require.NoError(t, err)
	require.Equal(t, bl.Header().ID().Bytes(), uint8Array[:])

	err = callContractAndGetOutput(abi, "blockID", toAddr, &uint8Array, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, b0.Header().ID().Bytes(), uint8Array[:])

	var uint64Output uint64
	err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(3))

	require.NoError(t, err)
	require.Equal(t, uint64(0), uint64Output)

	err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(2))
	assert.NoError(t, err)

	bl2, err := thorChain.GetTxBlock(id2)
	require.NoError(t, err)
	block2, err := thorChain.Repo().GetBlock(bl2.Header().ID())

	require.NoError(t, err)
	require.Equal(t, block2.Header().TotalScore(), uint64Output)

	err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(1))
	assert.NoError(t, err)
	block1, err := thorChain.Repo().GetBlock(bl.Header().ID())

	require.NoError(t, err)
	require.Equal(t, block1.Header().TotalScore(), uint64Output)

	err = callContractAndGetOutput(abi, "blockTotalScore", toAddr, &uint64Output, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, b0.Header().TotalScore(), uint64Output)

	err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(3))

	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).String(), bigIntOutput.String())

	err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(2))

	require.NoError(t, err)
	require.Equal(t, new(big.Int).SetUint64(block2.Header().Timestamp()), bigIntOutput)

	err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(1))

	require.NoError(t, err)
	require.Equal(t, new(big.Int).SetUint64(block1.Header().Timestamp()), bigIntOutput)

	err = callContractAndGetOutput(abi, "blockTime", toAddr, &bigIntOutput, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, new(big.Int).SetUint64(b0.Header().Timestamp()), bigIntOutput)

	var addressOutput common.Address
	err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(3))

	require.NoError(t, err)
	require.Equal(t, common.Address{}, addressOutput)

	err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(2))

	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())

	err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(1))

	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())

	err = callContractAndGetOutput(abi, "blockSigner", toAddr, &addressOutput, big.NewInt(0))

	require.NoError(t, err)
	require.Equal(t, common.Address{}, addressOutput)

	err = callContractAndGetOutput(abi, "txGasPayer", toAddr, &addressOutput)

	require.NoError(t, err)
	require.Equal(t, master1.Address.Bytes(), addressOutput.Bytes())
}
