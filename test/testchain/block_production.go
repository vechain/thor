// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"errors"
	"fmt"
	"math"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// MintBlock finds the validator with the earliest scheduled time, creates a block with the provided transactions, and adds it to the chain.
func (c *Chain) MintBlock(transactions ...*tx.Transaction) error {
	validator, found := c.NextValidator()
	if !found {
		return errors.New("no validator found")
	}

	best := c.repo.BestBlockSummary()
	now := best.Header.Timestamp() + thor.BlockInterval()
	p := packer.New(c.Repo(), c.Stater(), validator.Address, nil, c.GetForkConfig(), 0)
	flow, err := p.Schedule(c.Repo().BestBlockSummary(), now)
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

	return c.CommitBlock(newBlk, stage, receipts)
}

// CommitBlock manually adds a new block to the chain.
func (c *Chain) CommitBlock(newBlk *block.Block, stage *state.Stage, receipts tx.Receipts) error {
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

// NextValidator finds the validator with the earliest scheduled packing time and returns their account.
// You can remove this validator to simulate a validator going offline.
func (c *Chain) NextValidator() (genesis.DevAccount, bool) {
	var (
		when uint64 = math.MaxUint64
		acc  genesis.DevAccount
	)

	best := c.repo.BestBlockSummary()

	for i := range len(c.validators) {
		p := packer.New(c.Repo(), c.Stater(), c.validators[i].Address, nil, c.GetForkConfig(), 0)

		now := best.Header.Timestamp() + thor.BlockInterval()

		flow, err := p.Schedule(c.Repo().BestBlockSummary(), now)
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
