package node

import (
	"fmt"
	"time"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type Chain struct {
	db           *muxdb.MuxDB
	genesis      *genesis.Genesis
	engine       bft.Committer
	repo         *chain.Repository
	stater       *state.Stater
	genesisBlock *block.Block
}

func (c *Chain) Repo() *chain.Repository {
	return c.repo
}

func (c *Chain) Stater() *state.Stater {
	return c.stater
}

func (c *Chain) GenesisBlock() *block.Block {
	return c.genesisBlock
}

func (c *Chain) MintTransactions(transactions ...*tx.Transaction) error {
	for i, transaction := range transactions {
		err := c.commitTxOnChain(transaction, nil)
		if err != nil {
			return fmt.Errorf("unable to mint tx: %d : %w", i, err)
		}
	}
	return nil
}

func (c *Chain) MintTransactionsWithReceiptFunc(txAndRcpts ...*TxAndRcpt) error {
	for i, txAndRcpt := range txAndRcpts {
		err := c.commitTxOnChain(txAndRcpt.Transaction, txAndRcpt.ReceiptFunc)
		if err != nil {
			return fmt.Errorf("unable to mint tx: %d : %w", i, err)
		}
	}
	return nil
}

func (c *Chain) commitTxOnChain(transaction *tx.Transaction, receiptFunc func(tx.Receipts)) error {
	newBlk, stage, rcpts, err := c.packTxIntoBlock(transaction)
	if err != nil {
		return err
	}

	if receiptFunc != nil {
		receiptFunc(rcpts)
	}

	return c.commitTxToChain(newBlk, stage, rcpts)
}
func (c *Chain) packTxIntoBlock(transaction *tx.Transaction) (*block.Block, *state.Stage, tx.Receipts, error) {
	// TODO Fix the hardcoded accounts
	blkPacker := packer.New(c.Repo(), c.Stater(), genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	flow, err := blkPacker.Schedule(c.Repo().BestBlockSummary(), uint64(time.Now().Unix()))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to schedule tx: %w", err)
	}
	err = flow.Adopt(transaction)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to adopt tx: %w", err)
	}
	newBlk, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to pack tx: %w", err)
	}

	return newBlk, stage, receipts, nil
}

func (c *Chain) commitTxToChain(newBlk *block.Block, stage *state.Stage, receipts tx.Receipts) error {
	if _, err := stage.Commit(); err != nil {
		return fmt.Errorf("unable to commit tx: %w", err)
	}
	if err := c.Repo().AddBlock(newBlk, receipts, 0); err != nil {
		return fmt.Errorf("unable to add tx to repo: %w", err)
	}
	if err := c.Repo().SetBestBlockID(newBlk.Header().ID()); err != nil {
		return fmt.Errorf("unable to set best block: %w", err)
	}
	return nil
}
