// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pruner

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var log = log15.New("pkg", "pruner")

const (
	propsStoreName = "pruner.props"
	statusKey      = "status"

	stepInitiate           = ""
	stepArchiveIndexTrie   = "archiveIndexTrie"
	stepArchiveAccountTrie = "archiveAccountTrie"
	stepDropStale          = "dropStale"
)

// Pruner is the state pruner.
type Pruner struct {
	db     *muxdb.MuxDB
	repo   *chain.Repository
	ctx    context.Context
	cancel func()
	goes   co.Goes
}

// New creates and starts a state pruner.
func New(db *muxdb.MuxDB, repo *chain.Repository) *Pruner {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pruner{
		db:     db,
		repo:   repo,
		ctx:    ctx,
		cancel: cancel,
	}
	p.goes.Go(func() {
		if err := p.loop(); err != nil {
			if err != context.Canceled {
				log.Warn("pruner interrupted", "error", err)
			}
		}
	})
	return p
}

// Stop stops the state pruner.
func (p *Pruner) Stop() {
	p.cancel()
	p.goes.Wait()
}

func (p *Pruner) loop() error {
	var status status
	if err := status.Load(p.db); err != nil {
		return err
	}
	if status.Cycles == 0 && status.Step == stepInitiate {
		log.Info("pruner started")
	} else {
		log.Info("pruner started", "range", fmt.Sprintf("[%v, %v]", status.N1, status.N2), "step", status.Step)
	}

	pruner := p.db.NewTriePruner()

	if status.Cycles == 0 {
		if _, _, err := p.archiveIndexTrie(pruner, 0, 0); err != nil {
			return err
		}
		if _, _, _, _, err := p.archiveAccountTrie(pruner, 0, 0); err != nil {
			return err
		}
	}

	bestNum := func() uint32 {
		return p.repo.BestBlock().Header().Number()
	}

	waitUntil := func(n uint32) error {
		for {
			if bestNum() > n {
				return nil
			}
			select {
			case <-p.ctx.Done():
				return p.ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}

	for {
		switch status.Step {
		case stepInitiate:
			if err := pruner.SwitchLiveSpace(); err != nil {
				return err
			}
			status.N1 = status.N2
			status.N2 = bestNum() + 10
			// not necessary to prune if n2 is too small
			if status.N2 < thor.MaxStateHistory {
				status.N2 = thor.MaxStateHistory
			}
			if err := waitUntil(status.N2); err != nil {
				return err
			}
			log.Info("initiated", "range", fmt.Sprintf("[%v, %v]", status.N1, status.N2))
			status.Step = stepArchiveIndexTrie
		case stepArchiveIndexTrie:
			log.Info("archiving index trie...")
			nodeCount, entryCount, err := p.archiveIndexTrie(pruner, status.N1, status.N2)
			if err != nil {
				return err
			}
			log.Info("archived index trie", "nodes", nodeCount, "entries", entryCount)
			status.Step = stepArchiveAccountTrie
		case stepArchiveAccountTrie:
			log.Info("archiving account trie...")
			nodeCount, entryCount, sNodeCount, sEntryCount, err := p.archiveAccountTrie(pruner, status.N1, status.N2)
			if err != nil {
				return err
			}
			log.Info("archived account trie",
				"nodes", nodeCount, "entries", entryCount,
				"storageNodes", sNodeCount, "storageEntries", sEntryCount)
			status.Step = stepDropStale
		case stepDropStale:
			if err := waitUntil(status.N2 + thor.MaxStateHistory + 128); err != nil {
				return err
			}
			log.Info("sweeping stale nodes...")
			count, err := pruner.DropStaleNodes(p.ctx)
			if err != nil {
				return err
			}
			log.Info("swept stale nodes", "count", count)

			status.Cycles++
			status.Step = stepInitiate
		default:
			return fmt.Errorf("unexpected pruner step: %v", status.Step)
		}

		if err := status.Save(p.db); err != nil {
			return err
		}
	}
}

func (p *Pruner) archiveIndexTrie(pruner *muxdb.TriePruner, n1, n2 uint32) (nodeCount, entryCount int, err error) {
	var (
		bestChain              = p.repo.NewBestChain()
		id1, id2, root1, root2 thor.Bytes32
	)

	if id1, err = bestChain.GetBlockID(n1); err != nil {
		return
	}
	if _, root1, err = p.repo.GetBlockHeader(id1); err != nil {
		return
	}

	if id2, err = bestChain.GetBlockID(n2); err != nil {
		return
	}
	if _, root2, err = p.repo.GetBlockHeader(id2); err != nil {
		return
	}
	if n1 == 0 && n2 == 0 {
		root1 = thor.Bytes32{}
	}
	nodeCount, entryCount, err = pruner.ArchiveNodes(p.ctx, chain.IndexTrieName, root1, root2, nil)
	return
}

func (p *Pruner) archiveAccountTrie(pruner *muxdb.TriePruner, n1, n2 uint32) (nodeCount, entryCount, storageNodeCount, storageEntryCount int, err error) {
	var (
		bestChain        = p.repo.NewBestChain()
		header1, header2 *block.Header
		root1, root2     thor.Bytes32
	)

	if header1, err = bestChain.GetBlockHeader(n1); err != nil {
		return
	}
	if header2, err = bestChain.GetBlockHeader(n2); err != nil {
		return
	}
	root1, root2 = header1.StateRoot(), header2.StateRoot()
	if n1 == 0 && n2 == 0 {
		root1 = thor.Bytes32{}
	}

	nodeCount, entryCount, err = pruner.ArchiveNodes(p.ctx, state.AccountTrieName, root1, root2, func(key, blob1, blob2 []byte) error {
		var sRoot1, sRoot2 thor.Bytes32
		if len(blob1) > 0 {
			var acc state.Account
			if err := rlp.DecodeBytes(blob1, &acc); err != nil {
				return err
			}
			sRoot1 = thor.BytesToBytes32(acc.StorageRoot)
		}
		if len(blob2) > 0 {
			var acc state.Account
			if err := rlp.DecodeBytes(blob2, &acc); err != nil {
				return err
			}
			sRoot2 = thor.BytesToBytes32(acc.StorageRoot)
		}
		if sRoot1 != sRoot2 {
			n, e, err := pruner.ArchiveNodes(p.ctx, state.StorageTrieName(thor.BytesToBytes32(key)), sRoot1, sRoot2, nil)
			if err != nil {
				return err
			}
			storageNodeCount += n
			storageEntryCount += e
		}
		return nil
	})
	return
}
