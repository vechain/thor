package runtime_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestCall(t *testing.T) {
	kv, _ := lvldb.NewMem()

	b0, _, err := genesis.Mainnet.Build(state.NewCreator(kv))
	if err != nil {
		t.Fatal(err)
	}

	state, _ := state.New(b0.Header().StateRoot(), kv)

	rt := runtime.New(state,
		thor.Address{}, 0, 0, 0, func(uint32) thor.Hash { return thor.Hash{} })

	method := builtin.Params.ABI.MethodByName("executor")
	data, err := method.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	out := rt.Call(
		tx.NewClause(&builtin.Params.Address).WithData(data),
		0, math.MaxUint64, thor.Address{}, &big.Int{}, thor.Hash{})

	if out.VMErr != nil {
		t.Fatal(out.VMErr)
	}

	var addr common.Address
	if err := method.DecodeOutput(out.Value, &addr); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, thor.Address(addr), builtin.Executor.Address)
}

func TestExecuteTransaction(t *testing.T) {

	// kv, _ := lvldb.NewMem()

	// key, _ := crypto.GenerateKey()
	// addr1 := thor.Address(crypto.PubkeyToAddress(key.PublicKey))
	// addr2 := thor.BytesToAddress([]byte("acc2"))
	// balance1 := big.NewInt(1000 * 1000 * 1000)

	// b0, err := new(genesis.Builder).
	// 	Alloc(contracts.Energy.Address, &big.Int{}, contracts.Energy.RuntimeBytecodes()).
	// 	Alloc(addr1, balance1, nil).
	// 	Call(contracts.Energy.PackCharge(addr1, big.NewInt(1000000))).
	// 	Build(state.NewCreator(kv))

	// if err != nil {
	// 	t.Fatal(err)
	// }

	// tx := new(tx.Builder).
	// 	GasPrice(big.NewInt(1)).
	// 	Gas(1000000).
	// 	Clause(tx.NewClause(&addr2).WithValue(big.NewInt(10))).
	// 	Build()

	// sig, _ := crypto.Sign(tx.SigningHash().Bytes(), key)
	// tx = tx.WithSignature(sig)

	// state, _ := state.New(b0.Header().StateRoot(), kv)
	// rt := runtime.New(state,
	// 	thor.Address{}, 0, 0, 0, func(uint32) thor.Hash { return thor.Hash{} })
	// receipt, _, err := rt.ExecuteTransaction(tx)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// _ = receipt
	// assert.Equal(t, state.GetBalance(addr1), new(big.Int).Sub(balance1, big.NewInt(10)))
}
