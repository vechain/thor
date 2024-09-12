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

func (c *Chain) MintTransactions(account genesis.DevAccount, transactions ...*tx.Transaction) error {
	var txAndRcpts []*TxAndRcpt
	for _, transaction := range transactions {
		txAndRcpts = append(txAndRcpts, &TxAndRcpt{Transaction: transaction})
	}
	return c.MintTransactionsWithReceiptFunc(account, txAndRcpts...)
}

func (c *Chain) MintTransactionsWithReceiptFunc(account genesis.DevAccount, txAndRcpts ...*TxAndRcpt) error {
	return c.MintBlock(account, txAndRcpts...)
}

func (c *Chain) MintBlock(account genesis.DevAccount, txAndRcpts ...*TxAndRcpt) error {
	// create a new block
	blkPacker := packer.New(c.Repo(), c.Stater(), account.Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	blkFlow, err := blkPacker.Schedule(c.Repo().BestBlockSummary(), uint64(time.Now().Unix()))
	if err != nil {
		return fmt.Errorf("unable to schedule a new block: %w", err)
	}

	// adopt transactions in the new block
	for _, txAndRcpt := range txAndRcpts {
		if err = blkFlow.Adopt(txAndRcpt.Transaction); err != nil {
			return fmt.Errorf("unable to adopt tx into block: %w", err)
		}
	}

	// pack the transactions into the block
	newBlk, stage, receipts, err := blkFlow.Pack(account.PrivateKey, 0, false)
	if err != nil {
		return fmt.Errorf("unable to pack tx: %w", err)
	}

	// modify any receipts in the new block
	for _, txAndRcpt := range txAndRcpts {
		if txAndRcpt.ReceiptFunc != nil {
			txAndRcpt.ReceiptFunc(receipts)
		}
	}

	// commit block to chain state
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
