// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
)

func createDevAccounts(t *testing.T, accountNo int) []genesis.DevAccount {
	var accs []genesis.DevAccount

	for range accountNo {
		pk, err := crypto.GenerateKey()
		require.NoError(t, err)
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		accs = append(accs, genesis.DevAccount{Address: thor.Address(addr), PrivateKey: pk})
	}

	return accs
}

func testNode(t *testing.T) (*Node, error) {
	// create state accounts
	accounts := createDevAccounts(t, 5)

	// create test db
	db := muxdb.NewMem()

	// Initialize the test chain and dependencies
	thorChain, err := createChain(db, accounts)
	require.NoError(t, err)

	proposer := &accounts[0]

	engine, err := bft.NewEngine(thorChain.Repo(), thorChain.Database(), thorChain.GetForkConfig(), proposer.Address)
	require.NoError(t, err)

	masterAddr := &Master{
		PrivateKey: proposer.PrivateKey,
	}
	node := New(
		masterAddr,
		thorChain.Repo(),
		engine,
		thorChain.Stater(),
		nil, // *logdb.LogDB (not required for this test)
		nil, // TxPoolEngine (nil stub for test)
		"",
		nil, // CommunicatorEngine (nil stub for test)
		&thor.NoFork,
		Options{
			SkipLogs:         true,
			MinTxPriorityFee: 0,
			TargetGasLimit:   10_000_000,
		},
		consensus.New(thorChain.Repo(), thorChain.Stater(), &thor.NoFork),
		packer.New(thorChain.Repo(), thorChain.Stater(), masterAddr.Address(), masterAddr.Beneficiary, &thor.NoFork, 10_000_000),
	)
	return node, nil
}

// captureLogs temporarily replaces the global root logger with one that
// writes into an in-memory buffer and returns the buffer and a restore func.
// Use restore() in a defer to ensure the original logger is restored.
func captureLogs() (*bytes.Buffer, func()) {
	buf := new(bytes.Buffer)
	old := log.Root()
	h := log.JSONHandler(buf)
	newLogger := log.NewLogger(h)
	log.SetDefault(newLogger)
	return buf, func() { log.SetDefault(old) }
}

func TestNode_GuardBlockProcessing_NormalNewBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	node.maxBlockNum = uint32(1000)
	newBlockNum := uint32(1001)
	err = node.guardBlockProcessing(newBlockNum, func(conflicts uint32) error {
		// mock process function and return no error
		return nil
	})
	assert.NoError(t, err, "Normal new block should be processed without error")
	assert.Equal(t, node.maxBlockNum, newBlockNum, "maxBlockNum should be updated to new block number")
}

func TestNode_GuardBlockProcessing_FutureBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	node.maxBlockNum = uint32(1000)
	err = node.guardBlockProcessing(1005, func(conflicts uint32) error {
		// mock process function and return no error
		return nil
	})
	assert.ErrorContains(t, err, errBlockTemporaryUnprocessable.Error(), "Future block should return temporary unprocessable error")
}

func TestNode_GuardBlockProcessing_KnownBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	node.maxBlockNum = uint32(1000)
	newBlockNum := uint32(980)
	err = node.guardBlockProcessing(newBlockNum, func(conflicts uint32) error {
		// mock process function and return errKnownBlock
		return errKnownBlock
	})
	assert.ErrorContains(t, err, errKnownBlock.Error(), "Future block should return temporary unprocessable error")
	assert.Equal(t, node.maxBlockNum, uint32(1000), "maxBlockNum should remain unchanged for old block")
}
