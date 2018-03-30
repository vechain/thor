package app

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	Logdb "github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var boot1 = "enode://ec0ccfaeefa53c6a7ec73ca36940c911902d1f6f9da7567d05c44d1aa841b309260f7b228008331b61c8890ece0297eb0c3541af1a51fd5fcc749bee9104e64a@192.168.31.182:55555"
var boot2 = "enode://0cc5f5ffb5d9098c8b8c62325f3797f56509bff942704687b6530992ac706e2cb946b90a34f1f19548cd3c7baccbcaea354531e5983c7d1bc0dee16ce4b6440b@40.118.3.223:30305"

type newBlockEvent struct {
	Blk      *block.Block
	Receipts tx.Receipts
	Trunk    bool
}

type packedEvent struct {
	blk      *block.Block
	receipts tx.Receipts
	ack      chan struct{}
}

type App struct {
	ctx context.Context

	component struct {
		txpool       *txpool.TxPool
		p2pSrv       *p2psrv.Server
		communicator *comm.Communicator
		consensus    *consensus.Consensus
		packer       *packer.Packer
		chain        *chain.Chain
		logdb        *Logdb.LogDB
		rest         *http.Server
	}

	event struct {
		packedChan       chan *packedEvent
		newBlockChan     chan *newBlockEvent
		bestBlockUpdated chan struct{}
	}
}

func New(
	lv *lvldb.LevelDB,
	proposer thor.Address,
	logdb *Logdb.LogDB,
	nodeKey *ecdsa.PrivateKey,
	port string,
) (*App, error) {
	stateCreator := state.NewCreator(lv)
	genesisBlock, _, err := genesis.Dev.Build(stateCreator)
	if err != nil {
		return nil, err
	}

	app := new(App)
	app.event.bestBlockUpdated = make(chan struct{})
	app.event.newBlockChan = make(chan *newBlockEvent)
	app.event.packedChan = make(chan *packedEvent)
	app.component.logdb = logdb
	if app.component.chain, err = chain.New(lv, genesisBlock); err != nil {
		return nil, err
	}
	app.component.txpool = txpool.New(app.component.chain, stateCreator)
	app.component.communicator = comm.New(app.component.chain, app.component.txpool)
	app.component.consensus = consensus.New(app.component.chain, stateCreator)
	app.component.packer = packer.New(app.component.chain, stateCreator, proposer, proposer)
	app.component.rest = &http.Server{Handler: api.New(app.component.chain, stateCreator, app.component.txpool, logdb)}
	app.component.p2pSrv = p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     nodeKey,
			MaxPeers:       25,
			ListenAddr:     port,
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})

	return app, nil
}

func (a *App) Run(ctx context.Context, restfulport string, privateKey *ecdsa.PrivateKey) error {
	if err := a.component.p2pSrv.Start("thor@111111", a.component.communicator.Protocols()); err != nil {
		return err
	}
	defer a.component.p2pSrv.Stop()

	peerCh := make(chan *p2psrv.Peer)
	a.component.p2pSrv.SubscribePeer(peerCh)

	a.component.communicator.Start(peerCh)
	defer a.component.communicator.Stop()

	lsr, err := net.Listen("tcp", restfulport)
	if err != nil {
		return err
	}
	defer lsr.Close()

	a.ctx = ctx
	var goes co.Goes

	goes.Go(a.newTxLoop)
	goes.Go(a.broadcastTxLoop)
	goes.Go(a.consentLoop)
	goes.Go(func() {
		a.packLoop(privateKey)
	})
	goes.Go(func() {
		a.startRestful(lsr)
	})

	goes.Wait()

	return nil
}

func (a *App) broadcastTxLoop() {
	txCh := make(chan *tx.Transaction)
	sub := a.component.txpool.SubscribeNewTransaction(txCh)

	for {
		select {
		case <-a.ctx.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			a.component.communicator.BroadcastTx(tx)
		}
	}
}

func (a *App) newTxLoop() {
	txCh := make(chan *tx.Transaction)
	sub := a.component.communicator.SubscribeTx(txCh)

	for {
		select {
		case <-a.ctx.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			a.component.txpool.Add(tx)
		}
	}
}

type orphan struct {
	blk       *block.Block
	timestamp uint64 // 块成为 orpahn 的时间, 最多维持 5 分钟
}

func (a *App) consentLoop() {
	futures := newFutureBlocks()
	orphanMap := make(map[thor.Hash]*orphan)
	consent := func(blk *block.Block) error {
		return a.consent(blk, futures, orphanMap)
	}

	subChan := make(chan *block.Block, 100)
	sub := a.component.communicator.SubscribeBlock(subChan)
	ticker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			sub.Unsubscribe()
			return
		case <-ticker.C:
			if blk := futures.Pop(); blk != nil {
				consent(blk)
			}
		case blk := <-subChan:
			if err := consent(blk); err != nil {
				break
			}

			now := uint64(time.Now().Unix())
			for id, orphan := range orphanMap {
				if orphan.timestamp+300 >= now {
					err := consent(orphan.blk)
					if err != nil && consensus.IsParentNotFound(err) {
						continue
					}
				}
				delete(orphanMap, id)
			}
		case packed := <-a.event.packedChan:
			if trunk, err := a.component.consensus.IsTrunk(packed.blk.Header()); err == nil {
				a.updateChain(&newBlockEvent{
					Blk:      packed.blk,
					Trunk:    trunk,
					Receipts: packed.receipts,
				})
				packed.ack <- struct{}{}
			}
		}
	}
}

func (a *App) consent(blk *block.Block, futures *futureBlocks, orphanMap map[thor.Hash]*orphan) error {
	trunk, receipts, err := a.component.consensus.Consent(blk, uint64(time.Now().Unix()))
	if err != nil {
		log.Warn(fmt.Sprintf("received new block(#%v bad)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "err", err.Error())
		if consensus.IsFutureBlock(err) {
			futures.Push(blk)
		} else if consensus.IsParentNotFound(err) {
			id := blk.Header().ID()
			if _, ok := orphanMap[id]; !ok {
				orphanMap[id] = &orphan{blk: blk, timestamp: uint64(time.Now().Unix())}
			}
		}
		return err
	}

	a.updateChain(&newBlockEvent{
		Blk:      blk,
		Trunk:    trunk,
		Receipts: receipts,
	})

	return nil
}

func (a *App) packLoop(privateKey *ecdsa.PrivateKey) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	for {
		timer.Reset(1 * time.Second)

		select {
		case <-a.ctx.Done():
			return
		case <-a.event.bestBlockUpdated:
			break
		case <-timer.C:
			if a.component.communicator.IsSynced() {
				bestBlock, err := a.component.chain.GetBestBlock()
				if err != nil {
					log.Error("%v", err)
					return
				}

				now := uint64(time.Now().Unix())
				if ts, adopt, commit, err := a.component.packer.Prepare(bestBlock.Header(), now); err == nil {
					waitSec := ts - now
					log.Info(fmt.Sprintf("waiting to propose new block(#%v)", bestBlock.Header().Number()+1), "after", fmt.Sprintf("%vs", waitSec))

					waitTime := time.NewTimer(time.Duration(waitSec) * time.Second)
					defer waitTime.Stop()

					select {
					case <-waitTime.C:
						pendings, err := a.component.txpool.Sorted(txpool.Pending)
						if err != nil {
							break
						}
						for _, tx := range pendings {
							err := adopt(tx)
							if packer.IsGasLimitReached(err) {
								break
							}
							a.component.txpool.OnProcessed(tx.ID(), err)
						}

						blk, receipts, err := commit(privateKey)
						if err != nil {
							break
						}

						log.Info(fmt.Sprintf("proposed new block(#%v)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
						pe := &packedEvent{
							blk:      blk,
							receipts: receipts,
							ack:      make(chan struct{}),
						}
						a.event.packedChan <- pe
						<-pe.ack
					case <-a.event.bestBlockUpdated:
						break
					case <-a.ctx.Done():
						return
					}
				}
			} else {
				log.Warn("has not synced")
			}
		}
	}
}

func (a *App) updateChain(newBlk *newBlockEvent) {
	_, err := a.component.chain.AddBlock(newBlk.Blk, newBlk.Receipts, newBlk.Trunk)
	if err != nil {
		return
	}

	log.Info(
		fmt.Sprintf("received new block(#%v valid %v)", newBlk.Blk.Header().Number(), newBlk.Trunk),
		"id", newBlk.Blk.Header().ID(),
		"size", newBlk.Blk.Size(),
	)

	if newBlk.Trunk {
		select {
		case a.event.bestBlockUpdated <- struct{}{}:
		default:
		}
		a.component.communicator.BroadcastBlock(newBlk.Blk)
	}

	// 日志待写
}

func (a *App) startRestful(lsr net.Listener) {
	go func() {
		<-a.ctx.Done()
		a.component.rest.Shutdown(context.TODO())
	}()

	if err := a.component.rest.Serve(lsr); err != http.ErrServerClosed {
		log.Error(fmt.Sprintf("%v", err))
	}
}
