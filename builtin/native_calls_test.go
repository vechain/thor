// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/vechain/thor/kv"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"
)

var errReverted = errors.New("evm: execution reverted")

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
	provedWork *big.Int
	txID       thor.Bytes32
	blockRef   tx.BlockRef
	expiration uint32

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

func (c *ccase) Expiration(expiration uint32) *ccase {
	c.expiration = expiration
	return c
}
func (c *ccase) ShouldVMError(err error) *ccase {
	c.vmerr = err
	return c
}

func (c *ccase) ShouldLog(events ...*vm.Event) *ccase {
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
		0, math.MaxUint64, &xenv.TransactionContext{
			ID:         c.txID,
			Origin:     c.caller,
			GasPrice:   &big.Int{},
			ProvedWork: c.provedWork,
			BlockRef:   c.blockRef,
			Expiration: c.expiration})

	if constant || vmout.VMErr != nil {
		newStateRoot, err := c.rt.State().Stage().Hash()
		assert.Nil(t, err, "should hash state")
		assert.Equal(t, stateRoot, newStateRoot)
	}
	assert.Equal(t, c.vmerr, vmout.VMErr)

	if c.output != nil {
		out, err := method.EncodeOutput((*c.output)...)
		assert.Nil(t, err, "should encode output")
		assert.Equal(t, out, vmout.Data, "should match output")
	}

	if c.events != nil {
		assert.Equal(t, c.events, vmout.Events, "should match event")
	}

	assert.Nil(t, c.rt.State().Err(), "should no state error")

	c.output = nil
	c.vmerr = nil
	c.events = nil

	return c
}

func buildTestLogs(methodName string, contractAddr thor.Address, topics []thor.Bytes32, args ...interface{}) []*vm.Event {
	nativeABI := builtin.Prototype.EventABI

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

func buildGenesis(kv kv.GetPutter, proc func(state *state.State) error) *block.Block {
	blk, _, _ := new(genesis.Builder).
		Timestamp(uint64(time.Now().Unix())).
		State(proc).
		Build(state.NewCreator(kv))
	return blk
}

func TestParamsNative(t *testing.T) {
	executor := thor.BytesToAddress([]byte("e"))
	kv, _ := lvldb.NewMem()
	b0 := buildGenesis(kv, func(state *state.State) error {
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		return nil
	})
	c, _ := chain.New(kv, b0)
	st, _ := state.New(b0.Header().StateRoot(), kv)

	rt := runtime.New(c.NewSeeker(b0.Header().ID()), st, &xenv.BlockContext{})

	test := &ctest{
		rt:  rt,
		abi: builtin.Params.ABI,
		to:  builtin.Params.Address,
	}

	key := thor.BytesToBytes32([]byte("key"))
	value := big.NewInt(999)
	setEvent := func(key thor.Bytes32, value *big.Int) *vm.Event {
		ev, _ := builtin.Params.ABI.EventByName("Set")
		data, _ := ev.Encode(value)
		return &vm.Event{
			Address: builtin.Params.Address,
			Topics:  []thor.Bytes32{ev.ID(), key},
			Data:    data,
		}
	}

	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	test.Case("set", key, value).
		Caller(executor).
		ShouldLog(setEvent(key, value)).
		Assert(t)

	test.Case("set", key, value).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("get", key).
		ShouldOutput(value).
		Assert(t)

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
		executor  = thor.BytesToAddress([]byte("e"))
	)

	kv, _ := lvldb.NewMem()
	b0 := buildGenesis(kv, func(state *state.State) error {
		state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
		state.SetBalance(thor.Address(endorsor1), thor.InitialProposerEndorsement)
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		return nil
	})
	c, _ := chain.New(kv, b0)
	st, _ := state.New(b0.Header().StateRoot(), kv)

	rt := runtime.New(c.NewSeeker(b0.Header().ID()), st, &xenv.BlockContext{})

	addEvent := func(signer, endorsor thor.Address, identity thor.Bytes32) *vm.Event {
		ev, _ := builtin.Authority.ABI.EventByName("Add")
		data, _ := ev.Encode(endorsor, identity)
		return &vm.Event{
			Address: builtin.Authority.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(signer[:])},
			Data:    data,
		}
	}
	removeEvent := func(signer thor.Address) *vm.Event {
		ev, _ := builtin.Authority.ABI.EventByName("Remove")
		data, _ := ev.Encode()
		return &vm.Event{
			Address: builtin.Authority.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(signer[:])},
			Data:    data,
		}
	}

	test := &ctest{
		rt:     rt,
		abi:    builtin.Authority.ABI,
		to:     builtin.Authority.Address,
		caller: executor,
	}

	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	test.Case("add", signer1, endorsor1, identity1).
		ShouldLog(addEvent(signer1, endorsor1, identity1)).
		Assert(t)

	test.Case("add", signer1, endorsor1, identity1).
		Caller(thor.BytesToAddress([]byte("other"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("add", signer1, endorsor1, identity1).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("remove", signer1).
		ShouldLog(removeEvent(signer1)).
		Assert(t)

	test.Case("add", signer1, endorsor1, identity1).
		ShouldLog(addEvent(signer1, endorsor1, identity1)).
		Assert(t)

	test.Case("add", signer2, endorsor2, identity2).
		ShouldLog(addEvent(signer2, endorsor2, identity2)).
		Assert(t)

	test.Case("add", signer3, endorsor3, identity3).
		ShouldLog(addEvent(signer3, endorsor3, identity3)).
		Assert(t)

	test.Case("get", signer1).
		ShouldOutput(true, endorsor1, identity1, true).
		Assert(t)

	test.Case("first").
		ShouldOutput(signer1).
		Assert(t)

	test.Case("next", signer1).
		ShouldOutput(signer2).
		Assert(t)

	test.Case("next", signer2).
		ShouldOutput(signer3).
		Assert(t)

	test.Case("next", signer3).
		ShouldOutput(thor.Address{}).
		Assert(t)

}

func TestEnergyNative(t *testing.T) {
	var (
		addr   = thor.BytesToAddress([]byte("addr"))
		to     = thor.BytesToAddress([]byte("to"))
		master = thor.BytesToAddress([]byte("master"))
		eng    = big.NewInt(1000)
	)

	kv, _ := lvldb.NewMem()
	b0 := buildGenesis(kv, func(state *state.State) error {
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		state.SetMaster(addr, master)
		return nil
	})

	c, _ := chain.New(kv, b0)
	st, _ := state.New(b0.Header().StateRoot(), kv)

	st.SetEnergy(addr, eng, b0.Header().Timestamp())
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, eng)

	transferEvent := func(from, to thor.Address, value *big.Int) *vm.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Transfer")
		data, _ := ev.Encode(value)
		return &vm.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(from[:]), thor.BytesToBytes32(to[:])},
			Data:    data,
		}
	}
	approvalEvent := func(owner, spender thor.Address, value *big.Int) *vm.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Approval")
		data, _ := ev.Encode(value)
		return &vm.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(owner[:]), thor.BytesToBytes32(spender[:])},
			Data:    data,
		}
	}

	rt := runtime.New(c.NewSeeker(b0.Header().ID()), st, &xenv.BlockContext{Time: b0.Header().Timestamp()})
	test := &ctest{
		rt:     rt,
		abi:    builtin.Energy.ABI,
		to:     builtin.Energy.Address,
		caller: builtin.Energy.Address,
	}

	test.Case("name").
		ShouldOutput("VeThor").
		Assert(t)

	test.Case("decimals").
		ShouldOutput(uint8(18)).
		Assert(t)

	test.Case("symbol").
		ShouldOutput("VTHO").
		Assert(t)

	test.Case("totalSupply").
		ShouldOutput(eng).
		Assert(t)

	test.Case("totalBurned").
		ShouldOutput(&big.Int{}).
		Assert(t)

	test.Case("balanceOf", addr).
		ShouldOutput(eng).
		Assert(t)

	test.Case("transfer", to, big.NewInt(10)).
		Caller(addr).
		ShouldLog(transferEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("transfer", to, big.NewInt(10)).
		Caller(thor.BytesToAddress([]byte("some one"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("move", addr, to, big.NewInt(10)).
		Caller(addr).
		ShouldLog(transferEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("move", addr, to, big.NewInt(10)).
		Caller(thor.BytesToAddress([]byte("some one"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("approve", to, big.NewInt(10)).
		Caller(addr).
		ShouldLog(approvalEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("allowance", addr, to).
		ShouldOutput(big.NewInt(10)).
		Assert(t)

	test.Case("transferFrom", addr, to, big.NewInt(10)).
		ShouldLog(transferEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("transferFrom", addr, to, big.NewInt(10)).
		Caller(thor.BytesToAddress([]byte("some one"))).
		ShouldVMError(errReverted).
		Assert(t)

}

func TestPrototypeNative(t *testing.T) {
	var (
		acc1     = thor.BytesToAddress([]byte("acc1"))
		contract thor.Address

		master = thor.BytesToAddress([]byte("master"))

		user = thor.BytesToAddress([]byte("user"))

		credit       = big.NewInt(1000)
		recoveryRate = big.NewInt(10)
		sponsor      = thor.BytesToAddress([]byte("sponsor"))

		key   = thor.BytesToBytes32([]byte("account-key"))
		value = thor.BytesToBytes32([]byte("account-value"))
	)

	kv, _ := lvldb.NewMem()
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	st, _ := state.New(genesisBlock.Header().StateRoot(), kv)
	c, _ := chain.New(kv, genesisBlock)
	st.SetStorage(thor.Address(acc1), key, value)
	st.SetBalance(thor.Address(acc1), big.NewInt(1))

	rt := runtime.New(c.NewSeeker(genesisBlock.Header().ID()), st, &xenv.BlockContext{
		Time:   genesisBlock.Header().Timestamp(),
		Number: genesisBlock.Header().Number(),
	})

	code, _ := hex.DecodeString("60606040523415600e57600080fd5b603580601b6000396000f3006060604052600080fd00a165627a7a72305820edd8a93b651b5aac38098767f0537d9b25433278c9d155da2135efc06927fc960029")
	out := rt.Call(tx.NewClause(nil).WithData(code), 0, math.MaxUint64, &xenv.TransactionContext{
		ID:         thor.Bytes32{},
		Origin:     master,
		GasPrice:   &big.Int{},
		ProvedWork: &big.Int{}})
	contract = *out.ContractAddress

	energy := big.NewInt(1000)
	st.SetEnergy(acc1, energy, genesisBlock.Header().Timestamp())

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
		ShouldLog(buildTestLogs("$SetMaster", acc1, []thor.Bytes32{thor.BytesToBytes32(acc1[:])})...).
		Assert(t)

	test.Case("native_master", acc1).
		ShouldOutput(acc1).
		Assert(t)

	test.Case("native_hasCode", acc1).
		ShouldOutput(false).
		Assert(t)

	test.Case("native_hasCode", contract).
		ShouldOutput(true).
		Assert(t)

	test.Case("native_setUserPlan", contract, credit, recoveryRate).
		ShouldOutput().
		ShouldLog(buildTestLogs("$SetUserPlan", contract, nil, credit, recoveryRate)...).
		Assert(t)

	test.Case("native_userPlan", contract).
		ShouldOutput(credit, recoveryRate).
		Assert(t)

	test.Case("native_isUser", contract, user).
		ShouldOutput(false).
		Assert(t)

	test.Case("native_addUser", contract, user).
		ShouldOutput().
		ShouldLog(buildTestLogs("$AddRemoveUser", contract, []thor.Bytes32{thor.BytesToBytes32(user[:])}, true)...).
		Assert(t)

	test.Case("native_isUser", contract, user).
		ShouldOutput(true).
		Assert(t)

	test.Case("native_userCredit", contract, user).
		ShouldOutput(credit).
		Assert(t)

	test.Case("native_removeUser", contract, user).
		ShouldOutput().
		ShouldLog(buildTestLogs("$AddRemoveUser", contract, []thor.Bytes32{thor.BytesToBytes32(user[:])}, false)...).
		Assert(t)

	test.Case("native_userCredit", contract, user).
		ShouldOutput(&big.Int{}).
		Assert(t)

	test.Case("native_isSponsor", contract, sponsor).
		ShouldOutput(false).
		Assert(t)

	test.Case("native_sponsor", contract, sponsor, true).
		ShouldOutput().
		ShouldLog(buildTestLogs("$Sponsor", contract, []thor.Bytes32{thor.BytesToBytes32(sponsor.Bytes())}, true)...).
		Assert(t)

	test.Case("native_isSponsor", contract, sponsor).
		ShouldOutput(true).
		Assert(t)

	test.Case("native_currentSponsor", contract).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("native_selectSponsor", contract, sponsor).
		ShouldOutput().
		ShouldLog(buildTestLogs("$SelectSponsor", contract, []thor.Bytes32{thor.BytesToBytes32(sponsor[:])})...).
		Assert(t)

	test.Case("native_sponsor", contract, sponsor, false).
		ShouldOutput().
		Assert(t)

	test.Case("native_isSponsor", contract, sponsor).
		ShouldOutput(false).
		Assert(t)

	test.Case("native_storage", acc1, key).
		ShouldOutput(value).
		Assert(t)

	test.Case("native_storage", builtin.Prototype.Address, thor.Blake2b(contract.Bytes(), []byte("user-plan"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_storageAtBlock", builtin.Prototype.Address, thor.Blake2b(contract.Bytes(), []byte("user-plan")), uint32(0)).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_storageAtBlock", acc1, key, uint32(0)).
		ShouldOutput(value).
		Assert(t)

	test.Case("native_storageAtBlock", acc1, key, uint32(100)).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_balanceAtBlock", acc1, uint32(0)).
		ShouldOutput(big.NewInt(1)).
		Assert(t)

	test.Case("native_balanceAtBlock", acc1, uint32(100)).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_energyAtBlock", acc1, uint32(0)).
		ShouldOutput(energy).
		Assert(t)

	test.Case("native_energyAtBlock", acc1, uint32(100)).
		ShouldVMError(errReverted).
		Assert(t)

	assert.False(t, st.GetCodeHash(builtin.Prototype.Address).IsZero())

}

func TestPrototypeNativeWithLongerBlockNumber(t *testing.T) {
	var (
		acc1 = thor.BytesToAddress([]byte("acc1"))
		key  = thor.BytesToBytes32([]byte("acc1-key"))
	)

	kv, _ := lvldb.NewMem()
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)

	st, _ := state.New(c.BestBlock().Header().StateRoot(), kv)
	rt := runtime.New(c.NewSeeker(c.BestBlock().Header().ID()), st, &xenv.BlockContext{
		Number: thor.MaxBackTrackingBlockNumber + 2,
		Time:   genesisBlock.Header().Timestamp(),
	})

	test := &ctest{
		rt:     rt,
		abi:    builtin.Prototype.NativeABI(),
		to:     builtin.Prototype.Address,
		caller: builtin.Prototype.Address,
	}

	test.Case("native_storageAtBlock", acc1, key, uint32(0)).
		ShouldOutput(thor.Bytes32{}).
		Assert(t)

	test.Case("native_balanceAtBlock", acc1, uint32(0)).
		ShouldOutput(big.NewInt(0)).
		Assert(t)

	test.Case("native_energyAtBlock", acc1, uint32(0)).
		ShouldOutput(big.NewInt(0)).
		Assert(t)
}

func TestPrototypeNativeWithBlockNumber(t *testing.T) {
	var (
		acc1         = thor.BytesToAddress([]byte("acc1"))
		key          = thor.BytesToBytes32([]byte("acc1-key"))
		credit       = big.NewInt(1000)
		recoveryRate = big.NewInt(10)
	)

	kv, _ := lvldb.NewMem()
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	st, _ := state.New(genesisBlock.Header().StateRoot(), kv)
	c, _ := chain.New(kv, genesisBlock)
	launchTime := genesisBlock.Header().Timestamp()

	for i := 1; i < 100; i++ {
		st.SetStorage(acc1, key, thor.BytesToBytes32([]byte(fmt.Sprintf("acc1-value%d", i))))
		st.SetBalance(acc1, big.NewInt(int64(i)))
		st.SetEnergy(acc1, big.NewInt(int64(i)), launchTime+uint64(i)*10)
		stateRoot, _ := st.Stage().Commit()
		b := new(block.Builder).
			ParentID(c.BestBlock().Header().ID()).
			TotalScore(c.BestBlock().Header().TotalScore() + 1).
			Timestamp(launchTime + uint64(i)*10).
			StateRoot(stateRoot).
			Build()
		c.AddBlock(b, tx.Receipts{})
	}

	st, _ = state.New(c.BestBlock().Header().StateRoot(), kv)
	rt := runtime.New(c.NewSeeker(c.BestBlock().Header().ID()), st, &xenv.BlockContext{
		Number: c.BestBlock().Header().Number(),
		Time:   c.BestBlock().Header().Timestamp(),
	})

	test := &ctest{
		rt:     rt,
		abi:    builtin.Prototype.NativeABI(),
		to:     builtin.Prototype.Address,
		caller: builtin.Prototype.Address,
	}

	test.Case("native_setUserPlan", acc1, credit, recoveryRate).
		ShouldOutput().
		ShouldLog(buildTestLogs("$SetUserPlan", acc1, nil, credit, recoveryRate)...).
		Assert(t)

	test.Case("native_storageAtBlock", acc1, key, uint32(10)).
		ShouldOutput(thor.BytesToBytes32([]byte("acc1-value10"))).
		Assert(t)

	test.Case("native_storageAtBlock", acc1, key, uint32(99)).
		ShouldOutput(thor.BytesToBytes32([]byte("acc1-value99"))).
		Assert(t)

	test.Case("native_balanceAtBlock", acc1, uint32(10)).
		ShouldOutput(big.NewInt(10)).
		Assert(t)

	test.Case("native_balanceAtBlock", acc1, uint32(99)).
		ShouldOutput(big.NewInt(99)).
		Assert(t)

	test.Case("native_energyAtBlock", acc1, uint32(10)).
		ShouldOutput(big.NewInt(10)).
		Assert(t)

	test.Case("native_energyAtBlock", acc1, uint32(99)).
		ShouldOutput(big.NewInt(99)).
		Assert(t)

	test.Case("native_storageAtBlock", builtin.Prototype.Address, thor.Blake2b(acc1.Bytes(), []byte("user-plan")), uint32(99)).
		ShouldVMError(errReverted).
		Assert(t)
}

func newBlock(parent *block.Block, score uint64, timestamp uint64, privateKey *ecdsa.PrivateKey) *block.Block {
	b := new(block.Builder).ParentID(parent.Header().ID()).TotalScore(parent.Header().TotalScore() + score).Timestamp(timestamp).Build()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
	return b.WithSignature(sig)
}

func TestExtensionNative(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)
	gene, _ := genesis.NewDevnet()
	genesisBlock, _, _ := gene.Build(state.NewCreator(kv))
	c, _ := chain.New(kv, genesisBlock)
	st.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes())

	privKeys := make([]*ecdsa.PrivateKey, 2)

	for i := 0; i < 2; i++ {
		privateKey, _ := crypto.GenerateKey()
		privKeys[i] = privateKey
	}

	b0 := genesisBlock
	b1 := newBlock(b0, 123, 456, privKeys[0])
	b2 := newBlock(b1, 789, 321, privKeys[1])

	b1_singer, _ := b1.Header().Signer()
	b2_singer, _ := b2.Header().Signer()

	_, err := c.AddBlock(b1, nil)
	assert.Equal(t, err, nil)
	_, err = c.AddBlock(b2, nil)
	assert.Equal(t, err, nil)

	rt := runtime.New(c.NewSeeker(b2.Header().ID()), st, &xenv.BlockContext{Number: 2, Time: b2.Header().Timestamp(), TotalScore: b2.Header().TotalScore(), Signer: b2_singer})

	contract := builtin.Extension.Address

	value := []byte("extension")

	test := &ctest{
		rt:  rt,
		abi: builtin.Extension.NativeABI(),
	}

	test.Case("native_blake2b256", value).
		To(contract).Caller(contract).
		ShouldOutput(thor.Blake2b(value)).
		Assert(t)

	test.Case("native_tokenTotalSupply").
		To(contract).Caller(contract).
		ShouldOutput(builtin.Energy.Native(st, 0).TokenTotalSupply()).
		Assert(t)

	test.Case("native_transactionBlockRef").
		To(contract).Caller(contract).
		BlockRef(tx.NewBlockRef(1)).
		ShouldOutput(tx.NewBlockRef(1).Number()).
		Assert(t)

	test.Case("native_transactionExpiration").
		To(contract).Caller(contract).
		Expiration(uint32(100)).
		ShouldOutput(uint32(100)).
		Assert(t)

	test.Case("native_transactionProvedWork").
		To(contract).Caller(contract).
		ProvedWork(big.NewInt(1e12)).
		ShouldOutput(big.NewInt(1e12)).
		Assert(t)

	test.Case("native_transactionID").
		To(contract).Caller(contract).
		TxID(thor.BytesToBytes32([]byte("txID"))).
		ShouldOutput(thor.BytesToBytes32([]byte("txID"))).
		Assert(t)

	test.Case("native_blockID", uint32(3)).
		To(contract).Caller(contract).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_blockID", uint32(2)).
		To(contract).Caller(contract).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_blockID", uint32(1)).
		To(contract).Caller(contract).
		ShouldOutput(b1.Header().ID()).
		Assert(t)

	test.Case("native_blockID", uint32(0)).
		To(contract).Caller(contract).
		ShouldOutput(b0.Header().ID()).
		Assert(t)

	test.Case("native_blockTotalScore", uint32(3)).
		To(contract).Caller(contract).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_blockTotalScore", uint32(2)).
		To(contract).Caller(contract).
		ShouldOutput(b2.Header().TotalScore()).
		Assert(t)

	test.Case("native_blockTotalScore", uint32(1)).
		To(contract).Caller(contract).
		ShouldOutput(b1.Header().TotalScore()).
		Assert(t)

	test.Case("native_blockTotalScore", uint32(0)).
		To(contract).Caller(contract).
		ShouldOutput(b0.Header().TotalScore()).
		Assert(t)

	test.Case("native_blockTimestamp", uint32(3)).
		To(contract).Caller(contract).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_blockTimestamp", uint32(2)).
		To(contract).Caller(contract).
		ShouldOutput(b2.Header().Timestamp()).
		Assert(t)

	test.Case("native_blockTimestamp", uint32(1)).
		To(contract).Caller(contract).
		ShouldOutput(b1.Header().Timestamp()).
		Assert(t)

	test.Case("native_blockTimestamp", uint32(0)).
		To(contract).Caller(contract).
		ShouldOutput(b0.Header().Timestamp()).
		Assert(t)

	test.Case("native_blockSigner", uint32(3)).
		To(contract).Caller(contract).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("native_blockSigner", uint32(2)).
		To(contract).Caller(contract).
		ShouldOutput(b2_singer).
		Assert(t)

	test.Case("native_blockSigner", uint32(1)).
		To(contract).Caller(contract).
		ShouldOutput(b1_singer).
		Assert(t)
}
