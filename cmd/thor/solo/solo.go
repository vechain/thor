// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

var log = log15.New()

// Solo mode is the standalone client without p2p server
type Solo struct {
	chain       *chain.Chain
	txPool      *txpool.TxPool
	packer      *packer.Packer
	logDB       *logdb.LogDB
	bestBlockCh chan *block.Block
	onDemand    bool
}

// New returns Solo instance
func New(
	chain *chain.Chain,
	stateCreator *state.Creator,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	onDemand bool,
) *Solo {
	return &Solo{
		chain:    chain,
		txPool:   txPool,
		packer:   packer.New(chain, stateCreator, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address),
		logDB:    logDB,
		onDemand: onDemand,
	}
}

func (s *Solo) Run(ctx context.Context) error {
	goes := &co.Goes{}

	defer func() {
		<-ctx.Done()
		goes.Wait()
	}()

	goes.Go(func() {
		s.loop(ctx)
	})

	log.Info("prepared to pack block")

	return nil
}

func (s *Solo) loop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(10) * time.Second)
	defer ticker.Stop()

	var scope event.SubscriptionScope
	defer scope.Close()

	txEvCh := make(chan *txpool.TxEvent, 10)
	scope.Track(s.txPool.SubscribeTxEvent(txEvCh))

	s.packing(nil)

	for {
		select {
		case <-ctx.Done():
			log.Info("stopping interval packing service......")
			return
		case txEv := <-txEvCh:
			newTx := txEv.Tx
			singer, err := newTx.Signer()
			if err != nil {
				continue
			}
			log.Info("new Tx", "id", newTx.ID(), "signer", singer)
			if s.onDemand {
				s.packing(tx.Transactions{newTx})
			}
		case <-ticker.C:
			if s.onDemand {
				continue
			}
			s.packing(s.txPool.Executables())
		}
	}
}

func (s *Solo) packing(pendingTxs tx.Transactions) {

	best := s.chain.BestBlock()

	flow, err := s.packer.Mock(best.Header(), uint64(time.Now().Unix()))
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	for _, tx := range pendingTxs {
		err := flow.Adopt(tx)
		if err != nil {
			log.Error("executing transaction", "error", fmt.Sprintf("%+v", err.Error()))
		}
		switch {
		case packer.IsKnownTx(err) || packer.IsBadTx(err):
			s.txPool.Remove(tx.ID())
		case packer.IsGasLimitReached(err):
			break
		case packer.IsTxNotAdoptableNow(err):
			continue
		default:
			s.txPool.Remove(tx.ID())
		}
	}

	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}
	if _, err := stage.Commit(); err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	// If there is no tx packed in the on-demand mode then skip
	if s.onDemand && len(b.Transactions()) == 0 {
		return
	}

	blockID := b.Header().ID()
	log.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	log.Debug(b.String())

	batch := s.logDB.Prepare(b.Header())
	for i, tx := range b.Transactions() {
		origin, _ := tx.Signer()
		txBatch := batch.ForTransaction(tx.ID(), origin)
		receipt := receipts[i]
		for _, output := range receipt.Outputs {
			txBatch.Insert(output.Events, output.Transfers)
		}
	}
	if err := batch.Commit(); err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	// ignore fork when s
	_, err = s.chain.AddBlock(b, receipts)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}
}
