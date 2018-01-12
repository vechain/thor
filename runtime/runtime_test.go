package runtime_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestExecute(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := state.New(thor.Hash{}, kv)
	_, err := genesis.Build(state)
	if err != nil {
		t.Fatal(err)
	}

	rt := runtime.New(state, &block.Header{}, func(uint64) thor.Hash { return thor.Hash{} })

	addr := thor.BytesToAddress([]byte("acc1"))
	amount := big.NewInt(1000 * 1000 * 1000 * 1000)

	{
		// charge
		data, err := contracts.Energy.ABI.Pack(
			"charge",
			addr,
			amount,
		)
		if err != nil {
			t.Fatal(err)
		}

		out := rt.Execute(&tx.Clause{
			To:   &contracts.Energy.Address,
			Data: data,
		}, 0, 1000000, thor.GodAddress, new(big.Int), thor.Hash{})
		if out.VMErr != nil {
			t.Fatal(out.VMErr)
		}
	}
	{
		data, err := contracts.Energy.ABI.Pack(
			"balanceOf",
			addr,
		)
		if err != nil {
			t.Fatal(err)
		}

		out := rt.Execute(&tx.Clause{
			To:   &contracts.Energy.Address,
			Data: data,
		}, 0, 1000000, thor.GodAddress, new(big.Int), thor.Hash{})
		if out.VMErr != nil {
			t.Fatal(out.VMErr)
		}

		var retAmount *big.Int
		if err := contracts.Energy.ABI.Unpack(&retAmount, "balanceOf", out.Value); err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, amount, retAmount)
	}
}

func TestExecuteTransaction(t *testing.T) {

	kv, _ := lvldb.NewMem()
	state, _ := state.New(thor.Hash{}, kv)

	key, _ := crypto.GenerateKey()
	addr1 := thor.Address(crypto.PubkeyToAddress(key.PublicKey))
	addr2 := thor.BytesToAddress([]byte("acc2"))
	balance1 := big.NewInt(1000 * 1000 * 1000)

	_, err := new(genesis.Builder).
		Alloc(contracts.Energy.Address, &big.Int{}, contracts.Energy.RuntimeBytecodes()).
		Alloc(addr1, balance1, nil).
		Call(contracts.Energy.Address, func() []byte {
			data, err := contracts.Energy.ABI.Pack("charge", addr1, big.NewInt(1000000))
			if err != nil {
				panic(err)
			}
			return data
		}()).
		Build(state)

	if err != nil {
		t.Fatal(err)
	}

	tx := new(tx.Builder).
		GasPrice(big.NewInt(1)).
		Gas(1000000).
		Clause(&tx.Clause{
			To:    &addr2,
			Value: bn.FromBig(big.NewInt(10)),
		}).
		Build()

	signing := cry.NewSigning(thor.Hash{})
	sig, _ := signing.Sign(tx, crypto.FromECDSA(key))
	tx = tx.WithSignature(sig)

	rt := runtime.New(state, &block.Header{}, func(uint64) thor.Hash { return thor.Hash{} })
	receipt, _, err := rt.ExecuteTransaction(tx, signing)
	if err != nil {
		t.Fatal(err)
	}
	_ = receipt
	assert.Equal(t, state.GetBalance(addr1), new(big.Int).Sub(balance1, big.NewInt(10)))
}
