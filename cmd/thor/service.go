package main

import (
	"context"
	"crypto/ecdsa"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/cmd/thor/blockpool"
	"github.com/vechain/thor/cmd/thor/network"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/txpool"
)

type runner struct {
	destructor func()
	runner     func()
}

func (rur *runner) run(ctx context.Context) {
	go func() {
		defer rur.destructor()
		<-ctx.Done()
	}()

	rur.runner()
}

type service struct {
	chain           *chain.Chain
	stateC          *state.Creator
	bp              *blockpool.BlockPool
	ip              string
	nw              *network.Network
	bestBlockUpdate chan bool
}

func newService(chain *chain.Chain, stateC *state.Creator, nw *network.Network, ip string, bestBlockUpdate chan bool) *service {
	svr := &service{
		chain:           chain,
		stateC:          stateC,
		ip:              ip,
		bp:              blockpool.New(),
		nw:              nw,
		bestBlockUpdate: bestBlockUpdate}
	nw.Join(svr)
	return svr
}

func (svr *service) restful(rf *http.Server, listener net.Listener) *runner {
	return &runner{
		destructor: func() { rf.Shutdown(context.TODO()) },
		runner: func() {
			if err := rf.Serve(listener); err != http.ErrServerClosed {
				log.Fatalf("[%v restful]: %v\n", svr.ip, err)
			}

			log.Printf("[%v restful]: restfulService exit\n", svr.ip)
		}}

}

func (svr *service) consensus(cs *consensus.Consensus) *runner {
	ctx, cancel := context.WithCancel(context.Background())

	return &runner{
		destructor: func() {
			svr.bp.Close()
			cancel()
		},
		runner: func() {
			for {
				block, err := svr.bp.FrontBlock()
				if err != nil {
					log.Printf("[%v consensus]: consensusService exit\n", svr.ip)
					return
				}
				log.Printf("[%v consensus]: get a block form block pool\n", svr.ip)

				isTrunk, err := cs.Consent(block.Body, uint64(time.Now().Unix()))
				if err != nil {
					log.Printf("[%v consensus]: %v\n", svr.ip, err)
					if consensus.IsDelayBlock(err) {
						log.Printf("[%v consensus]: is a delay block\n", svr.ip)
						go func() {
							select {
							case <-time.After(15 * time.Second):
								log.Printf("[%v consensus]: the delay block re insert to bp\n", svr.ip)
								svr.bp.InsertBlock(block)
							case <-ctx.Done():
								return
							}
						}()
					}
					continue
				}

				svr.updateChain(block.Body, isTrunk, block.TTL-1, "consensus")

				if isTrunk {
					svr.bestBlockUpdate <- true
				}
			}
		}}
}

func (svr *service) packer(pk *packer.Packer, privateKey *ecdsa.PrivateKey) *runner {
	done := make(chan int, 1)
	txpl := txpool.NewTxPool(svr.chain)

	return &runner{
		destructor: func() { done <- 1 },
		runner: func() {
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
					block, _, err := pack(txpl.NewIterator())
					if err != nil {
						log.Fatalf("[%v packer]: %v\n", svr.ip, err)
					}

					sig, err := crypto.Sign(block.Header().SigningHash().Bytes(), privateKey)
					if err != nil {
						log.Fatalf("[%v packer]: %v\n", svr.ip, err)
					}

					block = block.WithSignature(sig)
					svr.updateChain(block, true, 10, "packer")
				}
			}
		}}
}

func (svr *service) updateChain(block *block.Block, isTrunk bool, ttl int, tag string) {
	if err := consensus.AddBlock(nil, svr.chain, isTrunk) err != nil {
		log.Fatalf("[%v %v]: %v\n", svr.ip, tag, err)
	} 

	log.Printf("[%v %v]: a block has add to chain, %v\n", svr.ip, tag, isTrunk)
	svr.BePacked(network.Block{
		Body: block,
		TTL:  ttl})
}

func (svr *service) BePacked(block network.Block) {
	svr.nw.Notify(svr, block)
}

func (svr *service) UpdateBlockPool(block network.Block) {
	svr.bp.InsertBlock(block)
}

func (svr *service) GetIP() string {
	return svr.ip
}
