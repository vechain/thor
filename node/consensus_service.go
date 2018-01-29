package node

import (
	"context"
	"log"
	"time"

	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
)

func consensusService(ctx context.Context, bestBlockUpdate chan bool, bp *blockPool, chain *chain.Chain, stateC stateCreater) {
	cs := consensus.New(chain, stateC)

	go func() {
		defer bp.close()
		<-ctx.Done()
	}()

	for {
		block, err := bp.frontBlock()
		if err != nil {
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

		if err = chain.AddBlock(&block, isTrunk); err != nil {
			log.Fatalln(err)
		}
		log.Printf("[consensus]: add block to chain\n")

		if isTrunk {
			bestBlockUpdate <- true
		}
	}
}
