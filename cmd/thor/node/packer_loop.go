// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// gasLimitSoftLimit is the soft limit of the adaptive block gaslimit.
const gasLimitSoftLimit uint64 = 21000000

func (n *Node) packerLoop(ctx context.Context) {
	log.Debug("enter packer loop")
	defer log.Debug("leave packer loop")

	log.Info("waiting for synchronization...")
	select {
	case <-ctx.Done():
		return
	case <-n.comm.Synced():
	}
	log.Info("synchronization process done")

	var (
		authorized bool
		ticker     = n.repo.NewTicker()
	)

	n.packer.SetTargetGasLimit(n.targetGasLimit)

	for {
		now := uint64(time.Now().Unix())

		if n.targetGasLimit == 0 {
			// no preset, use suggested
			suggested := n.bandwidth.SuggestGasLimit()
			// apply soft limit in adaptive mode
			if suggested > gasLimitSoftLimit {
				suggested = gasLimitSoftLimit
			}
			n.packer.SetTargetGasLimit(suggested)
		}

		flow, err := n.packer.Schedule(n.repo.BestBlock(), now)
		if err != nil {
			if authorized {
				authorized = false
				log.Warn("unable to pack block", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				continue
			}
		}

		if !authorized {
			authorized = true
			log.Info("prepared to pack block")
		}
		log.Debug("scheduled to pack block", "after", time.Duration(flow.When()-now)*time.Second)

		for {
			if n.timeToPack(flow) {
				if err := n.pack(ctx, flow); err != nil {
					if err == context.Canceled {
						return
					}
					log.Error("failed to pack block", "err", err)
				}
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second / 2):
				if n.needReSchedule(flow) {
					log.Debug("re-schedule packer due to new best block")
					goto RE_SCHEDULE
				}
			}
		}
	RE_SCHEDULE:
	}
}

func (n *Node) needReSchedule(flow *packer.Flow) bool {
	best := n.repo.BestBlock().Header()
	s1, _ := best.Signer()
	s2, _ := flow.ParentHeader().Signer()

	if flow.Number() < n.forkConfig.VIP193 {
		/* Before VIP193, re-schedule regarding the following two conditions:
		1. parent block needs to update and the new best is not proposed by the same one
		2. best block is better than the block to be proposed
		*/

		if (best.Number() == flow.ParentHeader().Number() && s1 != s2) ||
			best.TotalScore() > flow.TotalScore() {
			return true
		}
	}

	/* After VIP193, re-schedule when parent block changes.(prevent one proposer propose blocks with different ID at the same height)*/
	if s1 != s2 || flow.ParentHeader().Number() != best.Number() {
		return true
	}

	return false
}

func (n *Node) timeToPack(flow *packer.Flow) bool {
	nowTs := uint64(time.Now().Unix())
	// start immediately in post vip 193 stage, to allow more time for getting backer signature
	if flow.ParentHeader().Number() >= n.forkConfig.VIP193 {
		return nowTs+thor.BlockInterval >= flow.When()
	}
	// blockInterval/2 early to allow more time for processing txs
	return nowTs+thor.BlockInterval/2 >= flow.When()
}

func (n *Node) pack(ctx context.Context, flow *packer.Flow) error {
	txs := n.txPool.Executables()
	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			n.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	var scope event.SubscriptionScope
	defer scope.Close()

	for _, tx := range txs {
		if err := flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			txsToRemove = append(txsToRemove, tx)
		}
	}

	if flow.Number() >= n.forkConfig.VIP193 {
		draft, err := flow.Draft(n.master.PrivateKey)
		if err != nil {
			return nil
		}
		n.comm.BroadcastDraft(draft)
		newAccCh := make(chan *comm.NewAcceptedEvent)
		scope.Track(n.comm.SubscribeAccepted(newAccCh))

		deadline := time.NewTimer(time.Duration(thor.BlockInterval/2) * time.Second)
		defer deadline.Stop()

		alpha := append([]byte(nil), flow.Alpha()...)
		proposalHash := draft.Proposal.Hash()
		for {
			select {
			case ev := <-newAccCh:
				if flow.Number() >= n.forkConfig.VIP193 {
					if ev.ProposalHash == proposalHash {
						if validateBackerSignature(ev.Signature, flow, proposalHash, alpha); err != nil {
							log.Debug("failed to process backer signature", "err", err)
							continue
						}
					}
				}
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				goto NEXT
			}
		}
	NEXT:
	}

	newBlock, stage, receipts, err := flow.Pack(n.master.PrivateKey)
	if err != nil {
		return err
	}

	prevTrunk, curTrunk, err := n.commitBlock(stage, newBlock, receipts)
	if err != nil {
		return errors.WithMessage(err, "commit block")
	}

	n.processFork(prevTrunk, curTrunk)

	if prevTrunk.HeadID() != curTrunk.HeadID() {
		n.comm.BroadcastBlock(newBlock)
		log.Info("📦 new block packed",
			"txs", len(receipts),
			"mgas", float64(newBlock.Header().GasUsed())/1000/1000,
			"id", shortID(newBlock.Header().ID()),
		)
	}

	return nil
}

func validateBackerSignature(sig block.ComplexSignature, flow *packer.Flow, proposalHash thor.Bytes32, alpha []byte) (err error) {
	pub, err := crypto.SigToPub(proposalHash.Bytes(), sig.Signature())
	if err != nil {
		return
	}
	backer := thor.Address(crypto.PubkeyToAddress(*pub))

	if flow.IsBackerKnown(backer) {
		return errors.New("known backer")
	}

	if flow.GetAuthority(backer) == nil {
		return fmt.Errorf("backer: %v is not an authority", backer)
	}

	beta, err := ecvrf.NewSecp256k1Sha256Tai().Verify(pub, alpha, sig.Proof())
	if err != nil {
		return
	}
	if poa.EvaluateVRF(beta, flow.MaxBlockProposers()) {
		flow.AddBackerSignature(sig, beta, backer)
	} else {
		return fmt.Errorf("invalid proof from %v", backer)
	}
	return
}
