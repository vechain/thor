package node

import (
	"context"
	"log"

	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
)

func consensusService(ctx context.Context, bp *blockPool, chain *chain.Chain, stateC stateCreater) {
	cs := consensus.New(chain, stateC)

	go func() {
		defer bp.close()
		<-ctx.Done()
	}()

	for {
		block, err := bp.frontBlock()
		if err != nil {
			log.Println("consensusService exit")
			return
		}
		log.Println("[consensus]: get a block form block pool.")

		isTrunk, err := cs.Consent(&block)
		if err != nil {
			log.Println(err)
			if consensus.IsDelayBlock(err) {
				log.Println("[consensus]: is a delay block.")
				bp.insertBlock(block)
			}
			continue
		}

		if err = chain.AddBlock(&block, isTrunk); err != nil {
			log.Fatalln(err)
		}
	}
}
