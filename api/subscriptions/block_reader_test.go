// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func blockCache() *messageCache {
	cache, _ := newMessageCache(generateBlockMessage, 10)
	return cache
}

func TestBlockReader_Read(t *testing.T) {
	repo, generatedBlocks, _ := initChain(t)
	genesisBlk := generatedBlocks[0]
	newBlock := generatedBlocks[1]

	// Test case 1: Successful read next blocks
	br := newBlockReader(repo, genesisBlk.Header().ID(), blockCache())
	res, ok, err := br.Read()

	assert.NoError(t, err)
	assert.True(t, ok)
	resBlock := &BlockMessage{}
	assert.NoError(t, json.Unmarshal(res[0], resBlock))
	assert.Equal(t, newBlock.Header().Number(), resBlock.Number)
	assert.Equal(t, newBlock.Header().ParentID(), resBlock.ParentID)

	// Test case 2: There is no new block
	br = newBlockReader(repo, newBlock.Header().ID(), blockCache())
	res, ok, err = br.Read()

	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)

	// Test case 3: Error when reading blocks
	br = newBlockReader(repo, thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), blockCache())
	res, ok, err = br.Read()

	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func initChain(t *testing.T) (*chain.Repository, []*block.Block, *txpool.TxPool) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	txPool := txpool.New(repo, stater, txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tr := new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err := crypto.Sign(tr.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tr = tr.WithSignature(sig)
	packer := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, _ := repo.GetBlockSummary(b.Header().ID())
	flow, err := packer.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	err = flow.Adopt(tr)
	if err != nil {
		t.Fatal(err)
	}
	blk, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	insertMockOutputEvent(receipts)
	if err := repo.AddBlock(blk, receipts, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBestBlockID(blk.Header().ID()); err != nil {
		t.Fatal(err)
	}
	return repo, []*block.Block{b, blk}, txPool
}

// This is a helper function to forcly insert an event into the output receipts
func insertMockOutputEvent(receipts tx.Receipts) {
	oldReceipt := receipts[0]
	events := make(tx.Events, 0)
	events = append(events, &tx.Event{
		Address: thor.BytesToAddress([]byte("to")),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic"))},
		Data:    []byte("data"),
	})
	outputs := &tx.Output{
		Transfers: oldReceipt.Outputs[0].Transfers,
		Events:    events,
	}
	receipts[0] = &tx.Receipt{
		Reverted: oldReceipt.Reverted,
		GasUsed:  oldReceipt.GasUsed,
		Outputs:  []*tx.Output{outputs},
		GasPayer: oldReceipt.GasPayer,
		Paid:     oldReceipt.Paid,
		Reward:   oldReceipt.Reward,
	}
}
