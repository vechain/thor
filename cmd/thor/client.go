package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
)

type events struct {
	newBlockPacked  chan *block.Block
	newBlockAck     chan struct{}
	bestBlockUpdate chan struct{}
}

// Options for Client.
type Options struct {
	DataPath    string
	Bind        string
	Proposer    thor.Address
	Beneficiary thor.Address
	PrivateKey  *ecdsa.PrivateKey
}

// // Client is the abstraction of local node.
// type Client struct {
// 	op           Options
// 	genesisBuild func(*state.Creator) (*block.Block, []*tx.Log, error)
// 	goes         co.Goes
// }

// // NewClient is a factory for Client.
// func NewClient(op Options) *Client {
// 	return &Client{
// 		op:           op,
// 		genesisBuild: genesis.Mainnet.Build}
// }

// // New is a factory for Client.
// func NewTestClient(op Options) *Client {
// 	return &Client{
// 		op:           op,
// 		genesisBuild: genesis.Dev.Build}
// }

func Start(op Options, p2pSrv *p2psrv.Server) (func(), error) {
	if op.DataPath == "" {
		return nil, errors.New("open database")
	}

	stateCreator, ch, restful, listener, txIter, cm, close, err := prepare(op, p2pSrv)
	if err != nil {
		return nil, err
	}

	var goes co.Goes
	ctx, cancel := context.WithCancel(context.Background())

	goes.Go(func() {
		go func() {
			<-ctx.Done()
			restful.Shutdown(context.TODO())
		}()

		if err := restful.Serve(listener); err != http.ErrServerClosed {
			log.Error(fmt.Sprintf("%v", err))
		}
	})

	es := &events{
		newBlockPacked:  make(chan *block.Block),
		newBlockAck:     make(chan struct{}),
		bestBlockUpdate: make(chan struct{}),
	}

	goes.Go(func() {
		cs := consensus.New(ch, stateCreator)
		blockCh := make(chan *block.Block)
		sub := cm.SubscribeBlock(blockCh)

		for {
			select {
			case <-ctx.Done():
				sub.Unsubscribe()
				return
			default:
				es.consent(ctx, blockCh, cm, ch, cs)
			}
		}
	})

	goes.Go(func() {
		pk := packer.New(ch, stateCreator, op.Proposer, op.Beneficiary)
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Println(cm.IsSynced())
				if cm.IsSynced() {
					es.pack(ctx, ch, pk, txIter, op.PrivateKey)
				}
			}
		}
	})

	return func() {
		cancel()
		goes.Wait()
		close()
	}, nil
}

func prepare(op Options, p2pSrv *p2psrv.Server) (*state.Creator, *chain.Chain, *http.Server, net.Listener, *txpool.Iterator, *comm.Communicator, func(), error) {
	lv, err := lvldb.New(op.DataPath, lvldb.Options{})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	stateCreator := state.NewCreator(lv)

	genesisBlock, _, err := genesis.Dev.Build(stateCreator)
	if err != nil {
		lv.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	ch := chain.New(lv)
	if err := ch.WriteGenesis(genesisBlock); err != nil {
		lv.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	txpl := txpool.New()

	txIter, err := txpl.NewIterator(ch, stateCreator)
	if err != nil {
		lv.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	lsr, err := net.Listen("tcp", op.Bind)
	if err != nil {
		lv.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	log, err := logdb.New(op.DataPath + "/log.db")
	if err != nil {
		lv.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	cm := comm.New(ch, txpl)
	peerCh := make(chan *p2psrv.Peer)
	p2pSrv.SubscribePeer(peerCh)
	p2pSrv.Start("thor@111111", cm.Protocols())
	cm.Start(peerCh)

	return stateCreator, ch, &http.Server{Handler: api.NewHTTPHandler(ch, stateCreator, txpl, log)}, lsr, txIter, cm, func() {
		cm.Stop()
		p2pSrv.Stop()
		log.Close()
		lsr.Close()
		lv.Close()
	}, nil
}

func (es *events) consent(ctx context.Context, blockCh chan *block.Block, cm *comm.Communicator, ch *chain.Chain, cs *consensus.Consensus) {
	select {
	case blk := <-blockCh:
		if _, err := ch.GetBlockHeader(blk.Header().ID()); !ch.IsNotFound(err) {
			return
		}
		signer, _ := blk.Header().Signer()
		if trunk, _, err := cs.Consent(blk, uint64(time.Now().Unix())); err == nil {
			ch.AddBlock(blk, trunk)
			if trunk {
				log.Info(fmt.Sprintf("received new block(#%v trunk)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "proposer", signer)
				cm.BroadcastBlock(blk)
				select {
				case es.bestBlockUpdate <- struct{}{}:
				default:
				}
			} else {
				log.Info(fmt.Sprintf("received new block(#%v branch)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "proposer", signer)
			}
		} else {
			log.Warn(fmt.Sprintf("received new block(#%v bad)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "proposer", signer, "err", err.Error())
		}
	case blk := <-es.newBlockPacked:
		if trunk, err := cs.IsTrunk(blk.Header()); err == nil {
			ch.AddBlock(blk, trunk)
			if trunk {
				cm.BroadcastBlock(blk)
			}
			es.newBlockAck <- struct{}{}
		}
	case <-ctx.Done():
		return
	}
}

func (es *events) pack(
	ctx context.Context,
	ch *chain.Chain,
	pk *packer.Packer,
	txIter *txpool.Iterator,
	privateKey *ecdsa.PrivateKey) {

	bestBlock, err := ch.GetBestBlock()
	if err != nil {
		return
	}

	now := uint64(time.Now().Unix())
	if ts, adopt, commit, err := pk.Prepare(bestBlock.Header(), now); err == nil {
		waitSec := ts - now
		log.Info(fmt.Sprintf("waiting to propose new block(#%v)", bestBlock.Header().Number()+1), "after", fmt.Sprintf("%vs", waitSec))

		waitTime := time.NewTimer(time.Duration(waitSec) * time.Second)
		defer waitTime.Stop()

		select {
		case <-waitTime.C:
			for txIter.HasNext() {
				err := adopt(txIter.Next())
				if packer.IsGasLimitReached(err) {
					break
				}
			}

			if blk, _, err := commit(privateKey); err == nil {
				log.Info(fmt.Sprintf("proposed new block(#%v)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
				es.newBlockPacked <- blk
				<-es.newBlockAck
			}
		case <-es.bestBlockUpdate:
			return
		case <-ctx.Done():
			return
		}
	}
}
