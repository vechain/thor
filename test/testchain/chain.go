// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

// Chain represents the blockchain structure.
// It includes database (db), genesis information (genesis), consensus engine (engine),
// repository for blocks and state (repo), state manager (stater), and the genesis block (genesisBlock).
type Chain struct {
	db           *muxdb.MuxDB
	genesis      *genesis.Genesis
	engine       bft.Committer
	repo         *chain.Repository
	stater       *state.Stater
	genesisBlock *block.Block
	logDB        *logdb.LogDB
	forkConfig   *thor.ForkConfig
	validators   []genesis.DevAccount
}

func New(
	db *muxdb.MuxDB,
	gene *genesis.Genesis,
	engine bft.Committer,
	repo *chain.Repository,
	stater *state.Stater,
	genesisBlock *block.Block,
	logDB *logdb.LogDB,
	forkConfig *thor.ForkConfig,
) *Chain {
	return &Chain{
		db:           db,
		genesis:      gene,
		engine:       engine,
		repo:         repo,
		stater:       stater,
		genesisBlock: genesisBlock,
		logDB:        logDB,
		forkConfig:   forkConfig,
		validators:   genesis.DevAccounts(),
	}
}

// DefaultForkConfig enables all forks at block 0
var DefaultForkConfig = thor.ForkConfig{}

// NewDefault is a wrapper function that creates a Chain for testing with the default fork config.
func NewDefault() (*Chain, error) {
	return NewIntegrationTestChain(genesis.DevConfig{ForkConfig: &DefaultForkConfig}, 180)
}

// NewWithFork is a wrapper function that creates a Chain for testing with custom forkConfig.
func NewWithFork(forkConfig *thor.ForkConfig, epochLength uint32) (*Chain, error) {
	return NewIntegrationTestChain(genesis.DevConfig{ForkConfig: forkConfig}, epochLength)
}

// NewIntegrationTestChain is a convenience function that creates a Chain for testing.
// It uses an in-memory database, development network genesis, and a solo BFT engine.
func NewIntegrationTestChain(config genesis.DevConfig, epochLength uint32) (*Chain, error) {
	// If the launch time is not set, set it to the current time minus the current time aligned with the block interval
	gene, err := CreateGenesis(config, 10, epochLength, epochLength)
	if err != nil {
		return nil, fmt.Errorf("unable to create genesis: %w", err)
	}
	return NewIntegrationTestChainWithGenesis(gene, config.ForkConfig, epochLength)
}

func NewIntegrationTestChainWithGenesis(gene *genesis.Genesis, forkConfig *thor.ForkConfig, epochLength uint32) (*Chain, error) {
	// Initialize the database
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	prm := params.New(thor.BytesToAddress([]byte("params")), st)
	_ = staker.New(builtin.Staker.Address, st, prm, nil)

	// Create the state manager (Stater) with the initialized database.
	stater := state.NewStater(db)
	geneBlk, _, _, err := gene.Build(stater)
	if err != nil {
		return nil, err
	}

	// Create the repository which manages chain data, using the database and genesis block.
	repo, err := chain.NewRepository(db, geneBlk)
	if err != nil {
		return nil, err
	}

	// Create an inMemory logdb
	logDb, err := logdb.NewMem()
	if err != nil {
		return nil, err
	}

	return New(
		db,
		gene,
		bft.NewMockedEngine(geneBlk.Header().ID()),
		repo,
		stater,
		geneBlk,
		logDb,
		forkConfig,
	), nil
}

// Genesis returns the genesis information of the chain, which includes the initial state and configuration.
func (c *Chain) Genesis() *genesis.Genesis {
	return c.genesis
}

// Repo returns the blockchain's repository, which stores blocks and other data.
func (c *Chain) Repo() *chain.Repository {
	return c.repo
}

// State returns the current state at the best block of the chain.
func (c *Chain) State() *state.State {
	bestBlkSummary := c.Repo().BestBlockSummary()
	return c.Stater().NewState(bestBlkSummary.Root())
}

// Stater returns the current state manager of the chain, which is responsible for managing the state of accounts and other elements.
func (c *Chain) Stater() *state.Stater {
	return c.stater
}

// Engine returns the consensus engine responsible for the blockchain's consensus mechanism.
func (c *Chain) Engine() bft.Committer {
	return c.engine
}

// GenesisBlock returns the genesis block of the chain, which is the first block in the blockchain.
func (c *Chain) GenesisBlock() *block.Block {
	return c.genesisBlock
}

func (c *Chain) GetTxReceipt(txID thor.Bytes32) (*tx.Receipt, error) {
	return c.repo.NewBestChain().GetTransactionReceipt(txID)
}

func (c *Chain) GetTxBlock(txID *thor.Bytes32) (*block.Block, error) {
	_, meta, err := c.repo.NewBestChain().GetTransaction(*txID)
	if err != nil {
		return nil, err
	}
	block, err := c.repo.NewBestChain().GetBlock(meta.BlockNum)
	return block, err
}

// GetAllBlocks retrieves all blocks from the blockchain, starting from the best block and moving backward to the genesis block.
// It limits the retrieval time to 5 seconds to avoid excessive delays.
func (c *Chain) GetAllBlocks() ([]*block.Block, error) {
	bestBlkSummary := c.Repo().BestBlockSummary()
	var blks []*block.Block
	currBlockID := bestBlkSummary.Header.ID()
	startTime := time.Now()

	// Traverse the chain backwards until the genesis block is reached or timeout occurs.
	for {
		blk, err := c.repo.GetBlock(currBlockID)
		if err != nil {
			return nil, err
		}
		blks = append(blks, blk)

		// Stop when the genesis block is reached and reverse the slice to have genesis at position 0.
		if blk.Header().Number() == c.genesisBlock.Header().Number() {
			slices.Reverse(blks) // make sure genesis is at position 0
			return blks, err
		}
		currBlockID = blk.Header().ParentID()

		// Check if the retrieval process is taking too long (more than 5 seconds).
		if time.Since(startTime) > 5*time.Second {
			return nil, errors.New("taking more than 5 seconds to retrieve all blocks")
		}
	}
}

// BestBlock returns the current best (latest) block in the chain.
func (c *Chain) BestBlock() (*block.Block, error) {
	return c.Repo().GetBlock(c.Repo().BestBlockSummary().Header.ID())
}

// GetForkConfig returns the current fork configuration based on the ID of the genesis block.
func (c *Chain) GetForkConfig() *thor.ForkConfig {
	return c.forkConfig
}

// ChainTag returns the chain tag of the genesis block.
func (c *Chain) ChainTag() byte {
	return c.Repo().ChainTag()
}

// Database returns the current database.
func (c *Chain) Database() *muxdb.MuxDB {
	return c.db
}

// LogDB returns the current logdb.
func (c *Chain) LogDB() *logdb.LogDB {
	return c.logDB
}

// RemoveValidator removes a validator from the chain's validator set based on the provided address.
func (c *Chain) RemoveValidator(address thor.Address) {
	for i, v := range c.validators {
		if v.Address == address {
			c.validators = slices.Delete(c.validators, i, i+1)
			return
		}
	}
}

// AddValidator adds a validator to the chain's validator set.
func (c *Chain) AddValidator(validator genesis.DevAccount) {
	c.validators = append(c.validators, validator)
}
