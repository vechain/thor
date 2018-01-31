package node

import (
	"context"
	"crypto/ecdsa"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/node/blockpool"
	"github.com/vechain/thor/node/network"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
)

type service struct {
	ctx             context.Context
	chain           *chain.Chain
	stateC          *state.Creator
	bp              *blockpool.BlockPool
	ip              string
	nw              *network.Network
	bestBlockUpdate chan bool
	destructor      func()
	runner          func()
}

func newService(ctx context.Context, chain *chain.Chain, stateC *state.Creator, nw *network.Network, ip string, bestBlockUpdate chan bool) *service {
	svr := &service{
		ctx:             ctx,
		chain:           chain,
		stateC:          stateC,
		nw:              nw,
		ip:              ip,
		bp:              blockpool.New(),
		bestBlockUpdate: bestBlockUpdate}
	nw.Join(svr)
	return svr
}

func (svr *service) run() {
	go func() {
		defer svr.destructor()
		<-svr.ctx.Done()
	}()

	svr.runner()
}

func (svr *service) withRestful(rf *http.Server, listener net.Listener) *service {
	newSvr := &service{ctx: svr.ctx}
	newSvr.destructor = func() { rf.Shutdown(context.TODO()) }
	newSvr.runner = func() {
		if err := rf.Serve(listener); err != http.ErrServerClosed {
			log.Fatalf("[%v restful]: %v\n", svr.ip, err)
		}

		log.Printf("[%v restful]: restfulService exit\n", svr.ip)
	}
	return newSvr
}

func (svr *service) withConsensus(cs *consensus.Consensus) *service {
	newSvr := &service{ctx: svr.ctx}
	newSvr.destructor = func() { svr.bp.Close() }
	newSvr.runner = func() {
		for {
			block, err := svr.bp.FrontBlock()
			if err != nil {
				log.Printf("[%v consensus]: consensusService exit\n", svr.ip)
				return
			}
			log.Printf("[%v consensus]: get a block form block pool\n", svr.ip)

			isTrunk, err := cs.Consent(&block, uint64(time.Now().Unix()))
			if err != nil {
				log.Printf("[%v consensus]: %v\n", svr.ip, err)
				if consensus.IsDelayBlock(err) {
					log.Printf("[%v consensus]: is a delay block\n", svr.ip)
					//svr.bp.InsertBlock(block)
				}
				continue
			}

			if err = svr.chain.AddBlock(&block, isTrunk); err != nil {
				log.Fatalf("[%v consensus]: %v\n", svr.ip, err)
			}

			log.Printf("[%v consensus]: add block to chain %v\n", svr.ip, isTrunk)
			//svr.BePacked(block)
			if isTrunk {
				svr.bestBlockUpdate <- true
			}
		}
	}

	return newSvr
}

func (svr *service) withPacker(pk *packer.Packer, privateKey *ecdsa.PrivateKey) *service {
	newSvr := &service{ctx: svr.ctx}
	done := make(chan int, 1)
	newSvr.destructor = func() { done <- 1 }
	newSvr.runner = func() {
		for {
			best, err := svr.chain.GetBestBlock()
			if err != nil {
				log.Fatalf("[%v packer]: %v\n", svr.ip, err)
			}

			now := uint64(time.Now().Unix())
			ts, pack, err := pk.Prepare(best.Header(), now)
			if err != nil {
				log.Fatalf("[%v packer]: %v\n", svr.ip, err)
			}

			log.Printf("[%v packer]: will pack block after %v\n", svr.ip, time.Duration(ts-now)*time.Second)
			target := time.After(time.Duration(ts-now) * time.Second)

			select {
			case <-done:
				log.Printf("[%v packer]: packerService exit\n", svr.ip)
				return
			case <-svr.bestBlockUpdate:
				log.Printf("[%v packer]: best block has updated\n", svr.ip)
				continue
			case <-target:
				block, _, err := pack(&fakeTxFeed{})
				if err != nil {
					log.Fatalf("[%v packer]: %v\n", svr.ip, err)
				}

				sig, err := crypto.Sign(block.Header().SigningHash().Bytes(), privateKey)
				if err != nil {
					log.Fatalf("[%v packer]: %v\n", svr.ip, err)
				}

				block = block.WithSignature(sig)
				if err = svr.chain.AddBlock(block, true); err != nil {
					log.Fatalf("[%v packer]: %v\n", svr.ip, err)
				}

				log.Printf("[%v packer]: a block has packed\n", svr.ip)
				svr.BePacked(*block)
			}
		}
	}

	return newSvr
}

func (svr *service) BePacked(block block.Block) {
	svr.nw.Notify(svr, block)
}

func (svr *service) UpdateBlockPool(block block.Block) {
	svr.bp.InsertBlock(block)
}

func (svr *service) GetIP() string {
	return svr.ip
}
