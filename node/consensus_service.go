package node

import (
	"context"
	"fmt"
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
			fmt.Println("consensusService exit")
			return
		}

		isTrunk, err := cs.Consent(&block)
		if err != nil {
			if consensus.IsDelayBlock(err) {
				bp.insertBlock(block)
			}
			continue
		}

		if err = chain.AddBlock(&block, isTrunk); err != nil {
			log.Fatalln(err)
		}
	}
}
