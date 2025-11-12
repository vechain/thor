// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/test/datagen"

	"github.com/vechain/thor/v2/consensus"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
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
	if config.LaunchTime == 0 {
		now := uint64(time.Now().Unix())
		config.LaunchTime = now - now%thor.BlockInterval()
	}

	gene, err := CreateGenesis(config.ForkConfig, 10, epochLength, epochLength)
	if err != nil {
		return nil, fmt.Errorf("unable to create genesis: %w", err)
	}
	return NewIntegrationTestChainWithGenesis(gene, config.ForkConfig, epochLength)
}

func NewIntegrationTestChainWithGenesis(gene *genesis.Genesis, forkConfig *thor.ForkConfig, epochLength uint32) (*Chain, error) {
	// Initialize the database
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	if epochLength != thor.EpochLength() {
		thor.SetConfig(thor.Config{
			EpochLength: epochLength,
		})
	}

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

// MintClauses creates a transaction with the provided clauses and mints a block containing that transaction.
func (c *Chain) MintClauses(account genesis.DevAccount, clauses []*tx.Clause) error {
	builder := new(tx.Builder).GasPriceCoef(255).
		BlockRef(tx.NewBlockRef(c.Repo().BestBlockSummary().Header.Number())).
		Expiration(1000).
		ChainTag(c.Repo().ChainTag()).
		Gas(10e6).
		Nonce(datagen.RandUint64())

	for _, clause := range clauses {
		builder.Clause(clause)
	}

	tx := builder.Build()
	signature, err := crypto.Sign(tx.SigningHash().Bytes(), account.PrivateKey)
	if err != nil {
		return fmt.Errorf("unable to sign tx: %w", err)
	}
	tx = tx.WithSignature(signature)

	return c.MintBlock(tx)
}

// AddValidators creates an `addValidation` staker transaction for each validator and mints a block containing those transactions.
func (c *Chain) AddValidators() error {
	method, ok := builtin.Staker.ABI.MethodByName("addValidation")
	if !ok {
		return errors.New("unable to find addValidation method in staker ABI")
	}

	stakerTxs := make([]*tx.Transaction, len(c.validators))
	for i, val := range c.validators {
		callData, err := method.EncodeInput(val.Address, thor.LowStakingPeriod())
		if err != nil {
			return fmt.Errorf("unable to encode addValidation input: %w", err)
		}
		clause := tx.NewClause(&builtin.Staker.Address).WithData(callData).WithValue(staker.MinStake)

		trx := new(tx.Builder).
			GasPriceCoef(255).
			BlockRef(tx.NewBlockRef(c.Repo().BestBlockSummary().Header.Number())).
			Expiration(1000).
			ChainTag(c.Repo().ChainTag()).
			Gas(10e6).
			Nonce(datagen.RandUint64()).
			Clause(clause).
			Build()

		trx = tx.MustSign(trx, val.PrivateKey)
		stakerTxs[i] = trx
	}

	return c.MintBlock(stakerTxs...)
}

func (c *Chain) NextValidator() (genesis.DevAccount, bool) {
	var (
		when uint64 = math.MaxUint64
		acc  genesis.DevAccount
	)

	best := c.repo.BestBlockSummary()

	for i := range len(c.validators) {
		p := packer.New(c.Repo(), c.Stater(), c.validators[i].Address, nil, c.GetForkConfig(), 0)

		now := best.Header.Timestamp() + thor.BlockInterval()

		flow, _, err := p.Schedule(c.Repo().BestBlockSummary(), now)
		if err != nil {
			continue
		}

		if flow.When() < when {
			acc = genesis.DevAccounts()[i]
			when = flow.When()
		}
	}

	if when == math.MaxUint64 {
		return genesis.DevAccount{}, false
	}
	return acc, true
}

// MintBlock finds the validator with the earliest scheduled time, creates a block with the provided transactions, and adds it to the chain.
func (c *Chain) MintBlock(transactions ...*tx.Transaction) error {
	validator, found := c.NextValidator()
	if !found {
		return errors.New("no validator found")
	}

	best := c.repo.BestBlockSummary()
	now := best.Header.Timestamp() + thor.BlockInterval()
	p := packer.New(c.Repo(), c.Stater(), validator.Address, nil, c.GetForkConfig(), 0)
	flow, _, err := p.Schedule(c.Repo().BestBlockSummary(), now)
	if err != nil {
		return fmt.Errorf("unable to schedule packing: %w", err)
	}

	// Adopt the provided transactions into the block.
	for _, trx := range transactions {
		if err := flow.Adopt(trx); err != nil {
			return fmt.Errorf("unable to adopt tx into block: %w", err)
		}
	}

	// Pack the adopted transactions into a block.
	newBlk, stage, receipts, err := flow.Pack(validator.PrivateKey, 0, false)
	if err != nil {
		return fmt.Errorf("unable to pack tx: %w", err)
	}

	// run the block through consensus validation
	if _, _, err := consensus.New(c.repo, c.stater, c.forkConfig).Process(best, newBlk, flow.When(), 0); err != nil {
		return fmt.Errorf("unable to process block: %w", err)
	}

	return c.AddBlock(newBlk, stage, receipts)
}

// AddBlock manually adds a new block to the chain.
func (c *Chain) AddBlock(newBlk *block.Block, stage *state.Stage, receipts tx.Receipts) error {
	// Commit the new block to the chain's state.
	if _, err := stage.Commit(); err != nil {
		return fmt.Errorf("unable to commit tx: %w", err)
	}

	// Add the block to the repository.
	if err := c.Repo().AddBlock(newBlk, receipts, 0, true); err != nil {
		return fmt.Errorf("unable to add tx to repo: %w", err)
	}

	// Write the new block and receipts to the logdb.
	w := c.LogDB().NewWriter()
	if err := w.Write(newBlk, receipts); err != nil {
		return err
	}
	if err := w.Commit(); err != nil {
		return err
	}
	return nil
}

// ClauseCall executes contract call with clause referenced by the clauseIdx parameter, the rest of tx is passed as is.
func (c *Chain) ClauseCall(account genesis.DevAccount, trx *tx.Transaction, clauseIdx int) ([]byte, uint64, error) {
	ch := c.repo.NewBestChain()
	summary, err := c.repo.GetBlockSummary(ch.HeadID())
	if err != nil {
		return nil, 0, err
	}
	st := state.New(c.db, trie.Root{Hash: summary.Header.StateRoot(), Ver: trie.Version{Major: summary.Header.Number()}})
	rt := runtime.New(
		ch,
		st,
		&xenv.BlockContext{Number: summary.Header.Number(), Time: summary.Header.Timestamp(), TotalScore: summary.Header.TotalScore(), Signer: account.Address},
		c.forkConfig,
	)
	maxGas := uint64(math.MaxUint32)
	exec, _ := rt.PrepareClause(trx.Clauses()[clauseIdx],
		0, maxGas, &xenv.TransactionContext{
			ID:         trx.ID(),
			Origin:     account.Address,
			GasPrice:   &big.Int{},
			GasPayer:   account.Address,
			ProvedWork: trx.UnprovedWork(),
			BlockRef:   trx.BlockRef(),
			Expiration: trx.Expiration(),
		})

	out, _, err := exec()
	if err != nil {
		return nil, 0, err
	}
	if out.VMErr != nil {
		return nil, 0, out.VMErr
	}
	return out.Data, maxGas - out.LeftOverGas, err
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
