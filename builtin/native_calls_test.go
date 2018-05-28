// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
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
	events     []*vm.Event

	output *[]interface{}
	vmerr  error
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

func (c *ccase) ShouldVMError(err error) *ccase {
	c.vmerr = err
	return c
}

func (c *ccase) ShouldLog(events []*vm.Event) *ccase {
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
	stateRoot, err := c.rt.State().Stage().Hash()
	assert.Nil(t, err, "should hash state")

	data, err := method.EncodeInput(c.args...)
	assert.Nil(t, err, "should encode input")

	vmout := c.rt.Call(tx.NewClause(&c.to).WithData(data),
		0, math.MaxUint64, c.caller, &big.Int{}, thor.Bytes32{}, &big.Int{})

	if constant || vmout.VMErr != nil {
		newStateRoot, err := c.rt.State().Stage().Hash()
		assert.Nil(t, err, "should hash state")
		assert.Equal(t, stateRoot, newStateRoot)
	}

	// fmt.Println(vmout)
	assert.Equal(t, c.vmerr, vmout.VMErr)

	if c.output != nil {
		out, err := method.EncodeOutput((*c.output)...)
		assert.Nil(t, err, "should encode output")
		assert.Equal(t, out, vmout.Data, "should match output")
	}

	if c.events != nil {
		assert.Equal(t, c.events, vmout.Events, "should match event")
	}

	assert.Nil(t, c.rt.State().Error(), "should no state error")

	c.output = nil
	c.vmerr = nil
	c.events = nil

	return c
}

func buildTestLogs(methodName string, contractAddr thor.Address, topics []thor.Bytes32, args ...interface{}) []*vm.Event {
	nativeABI := builtin.Prototype.ThorLibABI

	mustEventByName := func(name string) *abi.Event {
		if event, found := nativeABI.EventByName(name); found {
			return event
		}
		panic("event not found")
	}

	methodEvent := mustEventByName(methodName)

	etopics := make([]thor.Bytes32, 0, len(topics)+1)
	etopics = append(etopics, methodEvent.ID())

	for _, t := range topics {
		etopics = append(etopics, t)
	}

	data, _ := methodEvent.Encode(args...)

	testLogs := []*vm.Event{
		&vm.Event{
			Address: contractAddr,
			Topics:  etopics,
			Data:    data,
		},
	}

	return testLogs
}

func TestParamsNative(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)
	st.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

	rt := runtime.New(c.NewSeeker(genesisBlock.Header().ID()), st, thor.Address{}, 0, 0, 0)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Params.NativeABI(),
		to:     builtin.Params.Address,
		caller: builtin.Params.Address,
	}

	key := thor.BytesToBytes32([]byte("key"))
	value := big.NewInt(999)

	cases := []*ccase{
		test.Case("native_getExecutor").
			ShouldOutput(builtin.Executor.Address).
			Assert(t),

		test.Case("native_set", key, value).
			Assert(t),

		test.Case("native_get", key).
			ShouldOutput(value).
			Assert(t),
	}

	for _, c := range cases {
		c.Caller(thor.BytesToAddress([]byte("other"))).
			ShouldVMError(evm.ErrExecutionReverted()).
			Assert(t)
	}
}

func TestAuthorityNative(t *testing.T) {
	var (
		signer1   = thor.BytesToAddress([]byte("signer1"))
		endorsor1 = thor.BytesToAddress([]byte("endorsor1"))
		identity1 = thor.BytesToBytes32([]byte("identity1"))

		signer2   = thor.BytesToAddress([]byte("signer2"))
		endorsor2 = thor.BytesToAddress([]byte("endorsor2"))
		identity2 = thor.BytesToBytes32([]byte("identity2"))

		signer3   = thor.BytesToAddress([]byte("signer3"))
		endorsor3 = thor.BytesToAddress([]byte("endorsor3"))
		identity3 = thor.BytesToBytes32([]byte("identity3"))
	)

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)
	st.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
	st.SetBalance(thor.Address(endorsor1), thor.InitialProposerEndorsement)
	builtin.Params.Native(st).Set(thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)

	rt := runtime.New(c.NewSeeker(genesisBlock.Header().ID()), st, thor.Address{}, 0, 0, 0)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Authority.NativeABI(),
		to:     builtin.Authority.Address,
		caller: builtin.Authority.Address,
	}
	cases := []*ccase{
		test.Case("native_getExecutor").
			ShouldOutput(builtin.Executor.Address).
			Assert(t),

		test.Case("native_add", signer1, endorsor1, identity1).
			ShouldOutput(true).
			Assert(t),
		test.Case("native_add", signer1, endorsor1, identity1).
			ShouldOutput(false).
			Assert(t),

		test.Case("native_remove", signer1).
			ShouldOutput(true).
			Assert(t),

		test.Case("native_add", signer1, endorsor1, identity1).
			ShouldOutput(true).
			Assert(t),

		test.Case("native_add", signer2, endorsor2, identity2).
			ShouldOutput(true).
			Assert(t),

		test.Case("native_add", signer3, endorsor3, identity3).
			ShouldOutput(true).
			Assert(t),

		test.Case("native_get", signer1).
			ShouldOutput(true, endorsor1, identity1, true).
			Assert(t),

		test.Case("native_first").
			ShouldOutput(signer1).
			Assert(t),

		test.Case("native_next", signer1).
			ShouldOutput(signer2).
			Assert(t),

		test.Case("native_next", signer2).
			ShouldOutput(signer3).
			Assert(t),

		test.Case("native_next", signer3).
			ShouldOutput(thor.Address{}).
			Assert(t),

		test.Case("native_isEndorsed", signer1).
			ShouldOutput(true).
			Assert(t),

		test.Case("native_isEndorsed", signer2).
			ShouldOutput(false).
			Assert(t),

		test.Case("native_isEndorsed", thor.BytesToAddress([]byte("not a signer"))).
			ShouldOutput(false).
			Assert(t),
	}

	for _, c := range cases {
		c.Caller(thor.BytesToAddress([]byte("other"))).
			ShouldVMError(evm.ErrExecutionReverted()).
			Assert(t)
	}
}

func TestEnergyNative(t *testing.T) {
	var (
		addr     = thor.BytesToAddress([]byte("addr"))
		valueAdd = big.NewInt(100)
		valueSub = big.NewInt(10)
	)

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)
	st.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())

	rt := runtime.New(c.NewSeeker(genesisBlock.Header().ID()), st, thor.Address{}, 0, 0, 0)
	test := &ctest{
		rt:     rt,
		abi:    builtin.Energy.NativeABI(),
		to:     builtin.Energy.Address,
		caller: builtin.Energy.Address,
	}

	cases := []*ccase{

		test.Case("native_getBalance", addr).
			ShouldOutput(&big.Int{}).
			Assert(t),

		test.Case("native_addBalance", addr, valueAdd).
			Assert(t),

		test.Case("native_getBalance", addr).
			ShouldOutput(valueAdd).
			Assert(t),

		test.Case("native_subBalance", addr, valueSub).
			ShouldOutput(true).
			Assert(t),

		test.Case("native_subBalance", addr, valueAdd).
			ShouldOutput(false).
			Assert(t),

		test.Case("native_getBalance", addr).
			ShouldOutput(new(big.Int).Sub(valueAdd, valueSub)).
			Assert(t),

		test.Case("native_getTotalSupply").
			ShouldOutput(new(big.Int)).
			Assert(t),

		test.Case("native_getTotalBurned").
			ShouldOutput(new(big.Int).Sub(valueSub, valueAdd)).
			Assert(t),
	}

	for _, c := range cases {
		c.Caller(thor.BytesToAddress([]byte("other"))).
			ShouldVMError(evm.ErrExecutionReverted()).
			Assert(t)
	}
}

func TestPrototypeNative(t *testing.T) {
	var (
		acc1     = thor.BytesToAddress([]byte("acc1"))
		acc2     = thor.BytesToAddress([]byte("acc2"))
		contract thor.Address

		master = thor.BytesToAddress([]byte("master"))

		user = thor.BytesToAddress([]byte("user"))

		credit       = big.NewInt(1000)
		recoveryRate = big.NewInt(10)
		sponsor      = thor.BytesToAddress([]byte("sponsor"))
	)

	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)
	st.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
	st.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes())

	rt := runtime.New(c.NewSeeker(genesisBlock.Header().ID()), st, thor.Address{}, 0, 0, 0)

	code, _ := hex.DecodeString("60606040523415600e57600080fd5b603580601b6000396000f3006060604052600080fd00a165627a7a72305820edd8a93b651b5aac38098767f0537d9b25433278c9d155da2135efc06927fc960029")
	out := rt.Call(tx.NewClause(nil).WithData(code), 0, math.MaxUint64, master, &big.Int{}, thor.Bytes32{}, &big.Int{})
	contract = *out.ContractAddress

	energy := big.NewInt(1000)
	st.SetEnergy(acc1, energy, 0)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Prototype.NativeABI(),
		to:     builtin.Prototype.Address,
		caller: builtin.Prototype.Address,
	}

	test.Case("native_master", acc1).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("native_master", contract).
		ShouldOutput(master).
		Assert(t)

	test.Case("native_setMaster", acc1, acc1).
		ShouldOutput().
		ShouldLog(buildTestLogs("$SetMaster", acc1, []thor.Bytes32{thor.BytesToBytes32(acc1[:])})).
		Assert(t)

	test.Case("native_hasCode", acc1).
		ShouldOutput(false).
		Assert(t)
	test.Case("native_hasCode", contract).
		ShouldOutput(true).
		Assert(t)

	test.Case("native_moveEnergyTo", acc1, acc2, big.NewInt(1)).
		ShouldOutput().
		Assert(t)

	test.Case("native_setUserPlan", contract, credit, recoveryRate).
		ShouldOutput().
		ShouldLog(buildTestLogs("$SetUserPlan", contract, nil, credit, recoveryRate)).
		Assert(t)

	test.Case("native_userPlan", contract).
		ShouldOutput(credit, recoveryRate).
		Assert(t)

	test.Case("native_isUser", contract, user).
		ShouldOutput(false).
		Assert(t)

	test.Case("native_addUser", contract, user).
		ShouldOutput().
		ShouldLog(buildTestLogs("$AddRemoveUser", contract, []thor.Bytes32{thor.BytesToBytes32(user[:])}, true)).
		Assert(t)

	test.Case("native_isUser", contract, user).
		ShouldOutput(true).
		Assert(t)

	test.Case("native_userCredit", contract, user).
		ShouldOutput(credit).
		Assert(t)

	test.Case("native_removeUser", contract, user).
		ShouldOutput().
		ShouldLog(buildTestLogs("$AddRemoveUser", contract, []thor.Bytes32{thor.BytesToBytes32(user[:])}, false)).
		Assert(t)

	test.Case("native_userCredit", contract, user).
		ShouldOutput(&big.Int{}).
		Assert(t)

	test.Case("native_isSponsor", contract, sponsor).
		ShouldOutput(false).
		Assert(t)

	test.Case("native_sponsor", contract, sponsor, true).
		ShouldOutput().
		ShouldLog(buildTestLogs("$Sponsor", contract, []thor.Bytes32{thor.BytesToBytes32(sponsor.Bytes())}, true)).
		Assert(t)

	test.Case("native_isSponsor", contract, sponsor).
		ShouldOutput(true).
		Assert(t)

	test.Case("native_currentSponsor", contract).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("native_selectSponsor", contract, sponsor).
		ShouldOutput().
		ShouldLog(buildTestLogs("$SelectSponsor", contract, []thor.Bytes32{thor.BytesToBytes32(sponsor[:])})).
		Assert(t)

	test.Case("native_sponsor", contract, sponsor, false).
		ShouldOutput().
		Assert(t)

	test.Case("native_isSponsor", contract, sponsor).
		ShouldOutput(false).
		Assert(t)

	assert.False(t, st.GetCodeHash(builtin.Prototype.Address).IsZero())
}

func TestExtensionNative(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)
	st.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes())

	rt := runtime.New(c.NewSeeker(genesisBlock.Header().ID()), st, thor.Address{}, 0, 0, 0)

	contract := builtin.Extension.Address

	value := []byte("extension")

	test := &ctest{
		rt:  rt,
		abi: builtin.Extension.NativeABI(),
	}

	var blockNum uint32 = 1234567
	var blockId thor.Bytes32

	binary.BigEndian.PutUint32(blockId[:4], blockNum)

	builtin.Extension.Native(st).SetBlockNumAndID(blockId)

	test.Case("native_blake2b256", value).
		To(contract).Caller(contract).
		ShouldOutput(thor.Blake2b(value)).
		Assert(t)
	test.Case("native_getBlockIDByNum", blockNum).
		To(contract).Caller(contract).
		ShouldOutput(blockId).
		Assert(t)

}
