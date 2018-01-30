package node

import (
	"context"
	"crypto/ecdsa"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/vechain/thor/packer"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
)

type service struct {
	ctx        context.Context
	chain      *chain.Chain
	destructor func()
	runner     func()
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
			log.Fatalln(err)
		}

		log.Println("restfulService exit")
	}
	return newSvr
}

func (svr *service) withConsensus(cs *consensus.Consensus, bestBlockUpdate chan bool, bp *blockPool) *service {
	newSvr := &service{ctx: svr.ctx}
	newSvr.destructor = func() { bp.close() }
	newSvr.runner = func() {
		for {
			block, err := bp.frontBlock()
			if err != nil {
				close(bestBlockUpdate)
				log.Printf("[consensus]: consensusService exit")
				return
			}
			log.Printf("[consensus]: get a block form block pool\n")

			isTrunk, err := cs.Consent(&block, uint64(time.Now().Unix()))
			if err != nil {
				log.Println(err)
				if consensus.IsDelayBlock(err) {
					log.Printf("[consensus]: is a delay block\n")
					bp.insertBlock(block)
				}
				continue
			}

			if err = svr.chain.AddBlock(&block, isTrunk); err != nil {
				log.Fatalln(err)
			}
			log.Printf("[consensus]: add block to chain\n")

			if isTrunk {
				bestBlockUpdate <- true
			}
		}
	}

	return newSvr
}

func (svr *service) withPacker(pk *packer.Packer, bestBlockUpdate chan bool, privateKey *ecdsa.PrivateKey) *service {
	newSvr := &service{ctx: svr.ctx}
	done := make(chan int, 1)
	newSvr.destructor = func() { done <- 1 }
	newSvr.runner = func() {
		for {
			best, err := svr.chain.GetBestBlock()
			if err != nil {
				log.Fatalln(err)
			}

			now := uint64(time.Now().Unix())
			ts, pack, err := pk.Prepare(best.Header(), now)
			if err != nil {
				log.Fatalln(err)
			}

			log.Printf("[packer]: will pack block after %v\n", time.Duration(ts-now)*time.Second)
			target := time.After(time.Duration(ts-now) * time.Second)

			select {
			case <-done:
				log.Printf("[packer]: packerService exit\n")
				return
			case <-bestBlockUpdate:
				log.Printf("[packer]: best block update\n")
				continue
			case <-target:
				block, _, err := pack(&fakeTxFeed{})
				if err != nil {
					log.Fatalln(err)
				}

				sig, err := crypto.Sign(block.Header().SigningHash().Bytes(), privateKey)
				if err != nil {
					log.Fatalln(err)
				}

				block = block.WithSignature(sig)

				if err = svr.chain.AddBlock(block, true); err != nil {
					log.Fatalln(err)
				}

				// 通知所有观察者
			}
		}
	}

	return newSvr
}
