package comm_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/txpool"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
)

var boot1 = "enode://ec0ccfaeefa53c6a7ec73ca36940c911902d1f6f9da7567d05c44d1aa841b309260f7b228008331b61c8890ece0297eb0c3541af1a51fd5fcc749bee9104e64a@192.168.31.182:55555"
var boot2 = "enode://0cc5f5ffb5d9098c8b8c62325f3797f56509bff942704687b6530992ac706e2cb946b90a34f1f19548cd3c7baccbcaea354531e5983c7d1bc0dee16ce4b6440b@40.118.3.223:30305"

var k1 = "9a6cbf7fe0aebf21c4242fe5f5243af42b026b3c30f3783a2ae4ace456224b8b"
var k2 = "7d6e0535c8f38c583c81e3654b1cc6428c69196f513dff8126aca84082649ef2"

func mustHexToECDSA(k string) *ecdsa.PrivateKey {
	pk, err := crypto.HexToECDSA(k)
	if err != nil {
		panic(err)
	}
	return pk
}

func TestComm(t *testing.T) {
	srv1 := p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     mustHexToECDSA(k1),
			MaxPeers:       25,
			ListenAddr:     ":40001",
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})

	srv2 := p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     mustHexToECDSA(k2),
			MaxPeers:       25,
			ListenAddr:     ":50001",
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})

	lv1, err := lvldb.NewMem()
	if err != nil {
		return
	}
	ch1 := chain.New(lv1)
	genesisBlk1, err := genesis.Dev.Build(state.NewCreator(lv1))
	if err != nil {
		return
	}
	ch1.WriteGenesis(genesisBlk1)
	cm1 := comm.New(ch1, txpool.New())
	cm1.Start(srv1, "thor@111111")
	defer cm1.Stop()

	lv2, err := lvldb.NewMem()
	if err != nil {
		return
	}
	ch2 := chain.New(lv2)
	genesisBlk2, err := genesis.Dev.Build(state.NewCreator(lv2))
	if err != nil {
		return
	}
	ch2.WriteGenesis(genesisBlk2)
	cm2 := comm.New(ch2, txpool.New())
	cm2.Start(srv2, "thor@111111")
	defer cm2.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(2 * time.Second)

		for {
			select {
			case <-ctx.Done():
			case <-ticker.C:
				blk := new(block.Builder).TotalScore(10).ParentID(genesisBlk1.Header().ID()).Build()
				ch1.AddBlock(blk, true)
				//cm1.BroadcastBlock(blk)
				fmt.Printf("[cm1] %v\n", srv1.Sessions())
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
			case blk := <-cm2.BlockCh:
				fmt.Printf("[cm2] %v\n", blk)
			}
		}
	}()

	<-time.After(time.Second * 17)
	cancel()
}
