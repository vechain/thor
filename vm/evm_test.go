package vm

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type Tracer interface {
	Logger
	SetContext(*Context)
	GetResult() (json.RawMessage, error)
	// Stop terminates execution of the tracer at the first opportune moment.
	Stop(err error)
}

type ctorFn func(json.RawMessage) (Tracer, error)
type jsCtorFn func(string, json.RawMessage) (Tracer, error)

type elem struct {
	ctor ctorFn
	isJS bool
}

var DefaultDirectory = directory{elems: make(map[string]elem)}

type directory struct {
	elems  map[string]elem
	jsEval jsCtorFn
}

func (d *directory) Register(name string, f ctorFn, isJS bool) {
	d.elems[name] = elem{ctor: f, isJS: isJS}
}

func (d *directory) New(name string, cfg json.RawMessage, allowCustom bool) (Tracer, error) {
	if elem, ok := d.elems[name]; ok {
		return elem.ctor(cfg)
	}
	// backward compatible, allow users emit "Tracer" suffix
	if elem, ok := d.elems[name+"Tracer"]; ok {
		return elem.ctor(cfg)
	}

	if allowCustom {
		// Assume JS code
		tracer, err := d.jsEval(name, cfg)
		if err != nil {
			return nil, errors.Wrap(err, "create custom tracer")
		}
		return tracer, nil
	} else {
		return nil, errors.New("unsupported tracer")
	}
}

func init() {
	DefaultDirectory.Register("noopTracer", newNoopTracer, false)
}

type noopTracer struct{}

func newNoopTracer(_ json.RawMessage) (Tracer, error) {
	return &noopTracer{}, nil
}

func (t *noopTracer) CaptureStart(env *EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
}

func (t *noopTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
}

func (t *noopTracer) CaptureState(pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, rData []byte, depth int, err error) {
}

func (t *noopTracer) CaptureFault(pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, depth int, err error) {
}

func (t *noopTracer) CaptureEnter(typ OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

func (t *noopTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
}

func (*noopTracer) CaptureClauseStart(gasLimit uint64) {}

func (*noopTracer) CaptureClauseEnd(restGas uint64) {}

func (t *noopTracer) SetContext(ctx *Context) {
}

func (t *noopTracer) GetResult() (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (t *noopTracer) Stop(err error) {
}

func setupEvmTestContract(codeAddr *common.Address) (*EVM, *Contract) {
	statedb := NoopStateDB{}

	tracer, err := DefaultDirectory.New("noopTracer", json.RawMessage(`{}`), false)
	if err != nil {
		panic("failed to create noopTracer: " + err.Error())
	}

	evmLogger, ok := tracer.(Logger)
	if !ok {
		panic("noopTracer does not implement vm.Logger")
	}

	evmConfig := Config{
		Tracer: evmLogger,
	}

	evm := NewEVM(Context{
		BlockNumber:        big.NewInt(1),
		GasPrice:           big.NewInt(1),
		CanTransfer:        NoopCanTransfer,
		Transfer:           NoopTransfer,
		NewContractAddress: newContractAddress,
	},
		statedb,
		&ChainConfig{ChainConfig: *params.TestChainConfig}, evmConfig)

	contract := &Contract{
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

	caller := AccountRef(common.HexToAddress("0x01"))
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

	caller := AccountRef(common.HexToAddress("0x01"))
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

	parentContract := NewContract(AccountRef(parentCallerAddress), AccountRef(parentCallerAddress), big.NewInt(2000), 5000)
	childContract := NewContract(parentContract, AccountRef(objectAddress), big.NewInt(2000), 5000)

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

	parentContract := NewContract(AccountRef(parentCallerAddress), AccountRef(parentCallerAddress), big.NewInt(2000), 5000)
	childContract := NewContract(parentContract, AccountRef(objectAddress), big.NewInt(2000), 5000)

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

	ret, addr, leftOverGas, err := evm.Create(AccountRef(parentCallerAddress), input, 1000000, big.NewInt(2000))

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

	ret, addr, leftOverGas, err := evm.Create2(AccountRef(parentCallerAddress), input, 10000, big.NewInt(2000), uint256.NewInt(10000))

	assert.Nil(t, err)
	assert.NotNil(t, addr)
	assert.Nil(t, ret)
	assert.NotNil(t, leftOverGas)
}
