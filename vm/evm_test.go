package vm_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/tracers"
	_ "github.com/vechain/thor/v2/tracers/native"
	"github.com/vechain/thor/v2/vm"
)

func newContractAddress(evm *vm.EVM, counter uint32) common.Address {
	return common.HexToAddress("0x012345657ABC")
}

func setupEvmTestContract(codeAddr *common.Address) (*vm.EVM, *vm.Contract) {

	statedb := vm.NoopStateDB{}

	tracer, err := tracers.DefaultDirectory.New("noopTracer", json.RawMessage(`{}`), false)
	if err != nil {
		panic("failed to create noopTracer: " + err.Error())
	}

	evmLogger, ok := tracer.(vm.Logger)
	if !ok {
		panic("noopTracer does not implement vm.Logger")
	}

	evmConfig := vm.Config{
		Tracer: evmLogger,
	}

	evm := vm.NewEVM(vm.Context{
		BlockNumber:        big.NewInt(1),
		GasPrice:           big.NewInt(1),
		CanTransfer:        vm.NoopCanTransfer,
		Transfer:           vm.NoopTransfer,
		NewContractAddress: newContractAddress,
	},
		statedb,
		&vm.ChainConfig{ChainConfig: *params.TestChainConfig}, evmConfig)

	contract := &vm.Contract{
		CallerAddress: common.HexToAddress("0x01"),
		Code:          []byte{0x60, 0x02, 0x5b, 0x00},
		CodeHash:      common.HexToHash("somehash"),
		CodeAddr:      codeAddr,
		Gas:           500000,
		DelegateCall:  true,
	}

	contractCode := []byte{0x60, 0x00}
	contract.SetCode(common.BytesToHash(contractCode), contractCode)

	return evm, contract
}

func TestCall(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	caller := vm.AccountRef(common.HexToAddress("0x01"))
	contractAddr := common.HexToAddress("0x1")
	input := []byte{}

	ret, leftOverGas, err := evm.Call(caller, contractAddr, input, 1000000, big.NewInt(100000))

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestCallCode(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	caller := vm.AccountRef(common.HexToAddress("0x01"))
	contractAddr := common.HexToAddress("0x1")
	input := []byte{}

	ret, leftOverGas, err := evm.CallCode(caller, contractAddr, input, 1000000, big.NewInt(100000))

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestDelegateCall(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01")
	objectAddress := common.HexToAddress("0x03")
	input := []byte{}

	parentContract := vm.NewContract(vm.AccountRef(parentCallerAddress), vm.AccountRef(parentCallerAddress), big.NewInt(2000), 5000)
	childContract := vm.NewContract(parentContract, vm.AccountRef(objectAddress), big.NewInt(2000), 5000)

	ret, leftOverGas, err := evm.DelegateCall(childContract, parentContract.CallerAddress, input, 1000000)

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestStaticCall(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01")
	objectAddress := common.HexToAddress("0x03")
	input := []byte{}

	parentContract := vm.NewContract(vm.AccountRef(parentCallerAddress), vm.AccountRef(parentCallerAddress), big.NewInt(2000), 5000)
	childContract := vm.NewContract(parentContract, vm.AccountRef(objectAddress), big.NewInt(2000), 5000)

	ret, leftOverGas, err := evm.StaticCall(childContract, parentContract.CallerAddress, input, 1000000)

	assert.Nil(t, err)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestCreate(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01234567A")
	input := []byte{}

	ret, addr, leftOverGas, err := evm.Create(vm.AccountRef(parentCallerAddress), input, 1000000, big.NewInt(2000))

	assert.Nil(t, err)
	assert.NotNil(t, addr)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}

func TestCreate2(t *testing.T) {
	codeAddr := common.BytesToAddress([]byte{1})
	evm, _ := setupEvmTestContract(&codeAddr)

	parentCallerAddress := common.HexToAddress("0x01234567A")
	input := []byte{}

	ret, addr, leftOverGas, err := evm.Create2(vm.AccountRef(parentCallerAddress), input, 10000, big.NewInt(2000), uint256.NewInt(10000))

	assert.Nil(t, err)
	assert.NotNil(t, addr)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}
