// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	Tx "github.com/vechain/thor/tx"
)

func init() {
	log15.Root().SetHandler(log15.DiscardHandler())
}

func newPool() *TxPool {
	kv, _ := lvldb.NewMem()
	chain := newChain(kv)
	return New(chain, state.NewCreator(kv), Options{
		Limit:           10,
		LimitPerAccount: 2,
		MaxLifetime:     time.Hour,
	})
}
func TestNewClose(t *testing.T) {
	pool := newPool()
	defer pool.Close()
}

func TestSubscribeNewTx(t *testing.T) {
	pool := newPool()
	defer pool.Close()

	b1 := new(block.Builder).
		ParentID(pool.chain.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.chain.GenesisBlock().Header().StateRoot()).
		Build()
	pool.chain.AddBlock(b1, nil)

	txCh := make(chan *TxEvent)

	pool.SubscribeTxEvent(txCh)

	tx := newTx(pool.chain.Tag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	assert.Nil(t, pool.Add(tx))

	v := true
	assert.Equal(t, &TxEvent{tx, &v}, <-txCh)
}

func TestWashTxs(t *testing.T) {
	pool := newPool()
	defer pool.Close()
	txs, _, err := pool.wash(pool.chain.BestBlock().Header())
	assert.Nil(t, err)
	assert.Zero(t, len(txs))
	assert.Zero(t, len(pool.Executables()))

	tx := newTx(pool.chain.Tag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	assert.Nil(t, pool.Add(tx))

	txs, _, err = pool.wash(pool.chain.BestBlock().Header())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx}, txs)

	b1 := new(block.Builder).
		ParentID(pool.chain.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.chain.GenesisBlock().Header().StateRoot()).
		Build()
	pool.chain.AddBlock(b1, nil)

	txs, _, err = pool.wash(pool.chain.BestBlock().Header())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx}, txs)
}

func TestAdd(t *testing.T) {
	pool := newPool()
	defer pool.Close()
	b1 := new(block.Builder).
		ParentID(pool.chain.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.chain.GenesisBlock().Header().StateRoot()).
		Build()
	pool.chain.AddBlock(b1, nil)
	acc := genesis.DevAccounts()[0]

	dupTx := newTx(pool.chain.Tag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	tests := []struct {
		tx     *tx.Transaction
		errStr string
	}{
		{newTx(pool.chain.Tag()+1, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), "bad tx: chain tag mismatch"},
		{dupTx, ""},
		{dupTx, ""},
	}

	for _, tt := range tests {
		err := pool.Add(tt.tx)
		if tt.errStr == "" {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.errStr, err.Error())
		}
	}

	raw, _ := hex.DecodeString("f8dc81b984aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e20860000006060608180830334508083bc614ec2017fb882bb61f654ee9868682314900fb8fb93ad2dae1f81370bded564101388ba80c64250e186e18d4861830ec8d5d30f1d03f8697bb03f5a3d9ea375862b79ec6ed1850173183f8edb80cfda71dbdd6b46004b084f4032fb9bf37dfc274c1324ac897c1f506aa907b6e6acc61c94dc5c42c05b762e7e0fc0670f0420689961e6269da7ab00")
	var badReserved *Tx.Transaction
	if err := rlp.DecodeBytes(raw, &badReserved); err != nil {
		t.Error(err)
	}

	tests = []struct {
		tx     *Tx.Transaction
		errStr string
	}{
		{newTx(pool.chain.Tag(), nil, 21000, tx.NewBlockRef(200), 100, nil, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(pool.chain.Tag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(pool.chain.Tag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(2), acc), "tx rejected: unsupported features"},
		{badReserved, "tx rejected: unknown reserved fields"},
	}

	for _, tt := range tests {
		err := pool.StrictlyAdd(tt.tx)
		if tt.errStr == "" {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.errStr, err.Error())
		}
	}
}

func TestBeforeVIP191Add(t *testing.T) {
	kv, _ := lvldb.NewMem()
	defer kv.Close()

	chain := newChain(kv)

	acc := genesis.DevAccounts()[0]

	thor.SetCustomNetForkConfig(chain.GenesisBlock().Header().ID(), thor.ForkConfig{0, 100})

	pool := New(chain, state.NewCreator(kv), Options{
		Limit:           10,
		LimitPerAccount: 2,
		MaxLifetime:     time.Hour,
	})
	defer pool.Close()

	err := pool.StrictlyAdd(newTx(pool.chain.Tag(), nil, 21000, tx.NewBlockRef(200), 100, nil, Tx.Features(1), acc))

	assert.Equal(t, "tx rejected: reserved fields not empty", err.Error())
}
