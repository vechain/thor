// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
)

// ChainBuilder is a builder pattern struct used to construct a Chain instance.
// It allows customization of the database, genesis configuration, and the consensus engine (BFT).
type ChainBuilder struct {
	dbFunc      func() *muxdb.MuxDB                        // Function to initialize the database.
	genesisFunc func() *genesis.Genesis                    // Function to initialize the genesis configuration.
	engineFunc  func(repo *chain.Repository) bft.Committer // Function to initialize the BFT consensus engine.
}

// NewIntegrationTestChain is a convenience function that creates a Chain for testing.
// It uses an in-memory database, development network genesis, and a solo BFT engine.
func NewIntegrationTestChain() (*Chain, error) {
	return new(ChainBuilder).
		WithDB(muxdb.NewMem).             // Use in-memory database.
		WithGenesis(genesis.NewDevnet).   // Use devnet genesis settings.
		WithBFTEngine(solo.NewBFTEngine). // Use solo BFT engine.
		Build()                           // Build and return the chain.
}

// WithDB sets the database initialization function in the builder.
// It returns the updated builder instance to allow method chaining.
func (b *ChainBuilder) WithDB(db func() *muxdb.MuxDB) *ChainBuilder {
	b.dbFunc = db
	return b
}

// WithGenesis sets the genesis initialization function in the builder.
// It allows custom genesis settings and returns the updated builder instance.
func (b *ChainBuilder) WithGenesis(genesisFunc func() *genesis.Genesis) *ChainBuilder {
	b.genesisFunc = genesisFunc
	return b
}

// WithBFTEngine sets the consensus engine initialization function in the builder.
// It allows custom consensus settings and returns the updated builder instance.
func (b *ChainBuilder) WithBFTEngine(engineFunc func(repo *chain.Repository) bft.Committer) *ChainBuilder {
	b.engineFunc = engineFunc
	return b
}

// Build finalizes the chain creation process by calling the configured functions.
// It sets up the database, genesis block, state manager, repository, and consensus engine.
func (b *ChainBuilder) Build() (*Chain, error) {
	// Initialize the database using the provided function.
	db := b.dbFunc()

	// Create the state manager (Stater) with the initialized database.
	stater := state.NewStater(db)

	// Initialize the genesis block using the provided genesis function.
	gene := b.genesisFunc()
	geneBlk, _, _, err := gene.Build(stater)
	if err != nil {
		return nil, err
	}

	// Create the repository which manages chain data, using the database and genesis block.
	repo, err := chain.NewRepository(db, geneBlk)
	if err != nil {
		return nil, err
	}

	// Return a new Chain instance with all components initialized.
	return &Chain{
		db:           db,                 // The initialized database.
		genesis:      gene,               // The genesis configuration.
		genesisBlock: geneBlk,            // The genesis block.
		repo:         repo,               // The chain repository.
		stater:       stater,             // The state manager.
		engine:       b.engineFunc(repo), // The consensus engine using the repository.
	}, nil
}
