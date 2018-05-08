package node

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
)

func (n *Node) waitForSynced(ctx context.Context) bool {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if n.comm.IsSynced() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (n *Node) packerLoop(ctx context.Context) {
	log.Debug("enter packer loop")
	defer log.Debug("leave packer loop")

	log.Info("waiting for synchronization...")
	// wait for synced
	if !n.waitForSynced(ctx) {
		return
	}
	log.Info("synchronization process done")

	var (
		authorized bool
		parent     = n.chain.BestBlock()
		flow       *packer.Flow
		err        error
		ticker     = time.NewTicker(time.Second)
	)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		best := n.chain.BestBlock()
		if best.Header().ID() != parent.Header().ID() {
			parent = best
			if flow != nil {
				flow = nil
				log.Debug("re-schedule packer due to new best block")
			}
			continue
		}

		now := uint64(time.Now().Unix())
		if flow != nil {
			if now >= flow.When() {
				n.pack(flow)
			}
			continue
		}

		if flow, err = n.packer.Schedule(parent.Header(), now); err != nil {
			if authorized {
				authorized = false
				log.Warn("unable to pack block", "err", err)
			}
			continue
		}
		if !authorized {
			authorized = true
			log.Info("prepared to pack block")
		}
		log.Debug("scheduled to pack block", "after", time.Duration(flow.When()-now)*time.Second)
	}
}

func (n *Node) pack(flow *packer.Flow) {
	txs := n.txPool.Pending()
	var txsToRemove []thor.Bytes32
	defer func() {
		for _, id := range txsToRemove {
			n.txPool.Remove(id)
		}
	}()

	startTime := mclock.Now()
	for _, tx := range txs {
		if err := flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			txsToRemove = append(txsToRemove, tx.ID())
		}
	}
	newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
	if err != nil {
		log.Error("failed to pack block", "err", err)
		return
	}

	elapsed := mclock.Now() - startTime
	n.goes.Go(func() { n.packedBlockFeed.Send(&packedBlockEvent{newBlock, stage, receipts, elapsed}) })

	if elapsed > 0 {
		gasUsed := newBlock.Header().GasUsed()
		// calc target gas limit only if gas used above third of gas limit
		if gasUsed > newBlock.Header().GasLimit()/3 {
			targetGasLimit := uint64(thor.TolerableBlockPackingTime) * gasUsed / uint64(elapsed)
			n.packer.SetTargetGasLimit(targetGasLimit)
			log.Debug("reset target gas limit", "value", targetGasLimit)
		}
	}
}
