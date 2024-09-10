package node

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
)

type Node struct {
	chain  *Chain
	router *mux.Router
}

func (n *Node) Router() http.Handler {
	return n.router
}

func (n *Node) GenerateNewBlock() error {
	packer := packer.New(n.chain.Repo(), n.chain.stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork) // todo this should pack with other accoutns

	sum := n.chain.repo.BestBlockSummary()
	flow, err := packer.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		return fmt.Errorf("unable to schedule new block")
	}
	blk, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		return fmt.Errorf("unable to pack new block")
	}
	if _, err := stage.Commit(); err != nil {
		return fmt.Errorf("unable to commit new block")
	}
	if err := n.chain.Repo().AddBlock(blk, receipts, 0); err != nil {
		return fmt.Errorf("unable to add new block to chain")
	}
	if err := n.chain.Repo().SetBestBlockID(blk.Header().ID()); err != nil {
		return fmt.Errorf("unable to set best block in chain")
	}
	return nil
}

func (n *Node) GetAllBlocks() ([]*block.Block, error) {
	bestBlkSummary := n.chain.Repo().BestBlockSummary()
	var blks []*block.Block
	currBlockID := bestBlkSummary.Header.ID()
	startTime := time.Now()
	for {
		blk, err := n.chain.Repo().GetBlock(currBlockID)
		if err != nil {
			return nil, err
		}
		blks = append(blks, blk)

		if blk.Header().Number() == n.chain.genesisBlock.Header().Number() {
			slices.Reverse(blks) // make sure genesis is at position 0
			return blks, err
		}
		currBlockID = blk.Header().ParentID()

		if time.Since(startTime) > 5*time.Second {
			return nil, errors.New("taking more than 5 seconds to retrieve all blocks")
		}
	}
}

func (n *Node) Chain() *Chain {
	return n.chain
}
