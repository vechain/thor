package node

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var log = log15.New("pkg", "node")

type Node struct {
	goes   co.Goes
	packer *packer.Packer
	cons   *consensus.Consensus

	bestBlockFeed   event.Feed
	packedBlockFeed event.Feed
	blockChunkCh    chan []*block.Block
	blockChunkAckCh chan error

	master *Master
	chain  *chain.Chain
	logDB  *logdb.LogDB
	txPool *txpool.TxPool
	comm   *comm.Communicator
}

func New(
	master *Master,
	chain *chain.Chain,
	stateCreator *state.Creator,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	comm *comm.Communicator,
) *Node {
	return &Node{
		packer:          packer.New(chain, stateCreator, master.Address(), master.Beneficiary),
		cons:            consensus.New(chain, stateCreator),
		blockChunkCh:    make(chan []*block.Block),
		blockChunkAckCh: make(chan error),
		master:          master,
		chain:           chain,
		logDB:           logDB,
		txPool:          txPool,
		comm:            comm,
	}
}

func (n *Node) Run(ctx context.Context) error {
	defer n.goes.Wait()

	n.goes.Go(func() { n.txLoop(ctx) })
	n.goes.Go(func() { n.packerLoop(ctx) })
	n.goes.Go(func() { n.consensusLoop(ctx) })

	return nil
}
func (n *Node) HandleBlockChunk(chunk []*block.Block) error {
	n.blockChunkCh <- chunk
	return <-n.blockChunkAckCh
}

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
	// wait for synced
	if !n.waitForSynced(ctx) {
		return
	}

	parent, err := n.chain.GetBestBlock()
	if err != nil {
		log.Error("failed to get best block", "err", err)
		return
	}

	var scope event.SubscriptionScope
	bestBlockCh := make(chan *bestBlockEvent)
	scope.Track(n.bestBlockFeed.Subscribe(bestBlockCh))

	defer scope.Close()

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		now := uint64(time.Now().Unix())
		if timestamp, adopt, commit, err := n.packer.Prepare(parent.Header(), now); err != nil {
			log.Warn("failed to prepare for packing", "err", err)
		} else {
			timer.Reset(time.Duration(timestamp-now) * time.Second)
			select {
			case <-ctx.Done():
				return
			case bestBlock := <-bestBlockCh:
				parent = bestBlock.Block
				continue
			case <-timer.C:
				n.pack(adopt, commit)
			}
		}

		select {
		case <-ctx.Done():
			return
		case bestBlock := <-bestBlockCh:
			parent = bestBlock.Block
			continue
		}
	}
}

func (n *Node) consensusLoop(ctx context.Context) {
	log.Debug("enter consensus loop")
	defer log.Debug("leave consensus loop")

	var scope event.SubscriptionScope
	packedBlockCh := make(chan *packedBlockEvent)
	newBlockCh := make(chan *comm.NewBlockEvent)

	scope.Track(n.packedBlockFeed.Subscribe(packedBlockCh))
	scope.Track(n.comm.SubscribeBlock(newBlockCh))
	defer scope.Close()

	futureBlocks := cache.NewPrioCache(256)

	ticker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case packedBlock := <-packedBlockCh:
			if err := n.insertBlock(packedBlock.Block, packedBlock.receipts); err != nil {
				log.Error("failed to insert block", "err", err)
			}
		case newBlock := <-newBlockCh:
			if err := n.processBlock(newBlock.Block); err != nil {
				switch {
				case consensus.IsFutureBlock(err) || consensus.IsParentNotFound(err):
					futureBlocks.Set(newBlock.Header().ID(), newBlock, -float64(newBlock.Header().Number()))
				case consensus.IsKnownBlock(err):
				default:
					log.Error("failed to import block", "err", err)
				}
			}
		case blocks := <-n.blockChunkCh:
			n.blockChunkAckCh <- func() error {
				for _, block := range blocks {
					if err := n.processBlock(block); err != nil {
						log.Error("failed to import downloaded block", "err", err)
						return err
					}

					select {
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}()
		}
	}
}

func (n *Node) pack(adopt packer.Adopt, commit packer.Commit) {
	startTime := mclock.Now()
	for _, tx := range n.txPool.Pending() {
		err := adopt(tx)
		switch {
		case packer.IsGasLimitReached(err):
			break
		case packer.IsTxNotAdoptableNow(err):
			continue
		default:
			n.txPool.Remove(tx.ID())
		}
	}
	newBlock, receipts, err := commit(n.master.PrivateKey)
	if err != nil {
		log.Error("failed to pack block", "err", err)
		return
	}

	if elapsed := mclock.Now() - startTime; elapsed > 0 {
		gasUsed := newBlock.Header().GasUsed()
		// calc target gas limit only if gas used above third of gas limit
		if gasUsed > newBlock.Header().GasLimit()/3 {
			targetGasLimit := uint64(thor.TolerableBlockPackingTime) * gasUsed / uint64(elapsed)
			n.packer.SetTargetGasLimit(targetGasLimit)
		}
	}

	n.goes.Go(func() { n.packedBlockFeed.Send(&packedBlockEvent{newBlock, receipts}) })
}

func (n *Node) processBlock(blk *block.Block) error {
	now := uint64(time.Now().Unix())
	receipts, err := n.cons.Process(blk, now)
	if err != nil {
		return err
	}

	return n.insertBlock(blk, receipts)
}

func (n *Node) insertBlock(newBlock *block.Block, receipts tx.Receipts) error {
	isTrunk, err := n.cons.IsTrunk(newBlock.Header())
	if err != nil {
		return err
	}

	fork, err := n.chain.AddBlock(newBlock, receipts, isTrunk)
	if err != nil {
		return err
	}

	forkIDs := make([]thor.Bytes32, 0, len(fork.Branch))
	for _, block := range fork.Branch {
		forkIDs = append(forkIDs, block.Header().ID())
		for _, tx := range block.Transactions() {
			n.txPool.Add(tx)
		}
	}

	batch := n.logDB.Prepare(newBlock.Header())
	for i, tx := range newBlock.Transactions() {
		origin, _ := tx.Signer()
		txBatch := batch.ForTransaction(tx.ID(), origin)
		receipt := receipts[i]

		for _, output := range receipt.Outputs {
			txBatch.Insert(output.Events, output.Transfers)
		}
	}

	if err := batch.Commit(forkIDs...); err != nil {
		return err
	}
	if isTrunk {
		n.bestBlockFeed.Send(&bestBlockEvent{newBlock})
		n.comm.BroadcastBlock(newBlock)
	}
	return nil
}

func (n *Node) txLoop(ctx context.Context) {
	log.Debug("enter tx loop")
	defer log.Debug("leave tx loop")

	txPoolCh := make(chan *tx.Transaction)
	commTxCh := make(chan *comm.NewTransactionEvent)
	var scope event.SubscriptionScope
	scope.Track(n.txPool.SubscribeNewTransaction(txPoolCh))
	scope.Track(n.comm.SubscribeTransaction(commTxCh))
	defer scope.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case tx := <-txPoolCh:
			n.comm.BroadcastTx(tx)
		case tx := <-commTxCh:
			n.txPool.Add(tx.Transaction)
		}
	}
}
