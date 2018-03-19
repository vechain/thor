package comm_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/txpool"
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

func makeAComm(key string, port string) (*comm.Communicator, *p2psrv.Server, *chain.Chain, *block.Block) {
	srv := p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     mustHexToECDSA(key),
			MaxSessions:    25,
			ListenAddr:     port,
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})

	lv, err := lvldb.NewMem()
	if err != nil {
		return nil, nil, nil, nil
	}
	ch := chain.New(lv)
	genesisBlk, err := genesis.Dev.Build(state.NewCreator(lv))
	if err != nil {
		return nil, nil, nil, nil
	}
	ch.WriteGenesis(genesisBlk)
	return comm.New(ch, txpool.New()), srv, ch, genesisBlk
}

func TestSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cm1, srv1, ch1, genesisBlk := makeAComm(k1, ":40001")
	sessionCh1 := make(chan *p2psrv.Session)
	srv1.SubscribeSession(sessionCh1)
	srv1.Start("thor@111111", cm1.Protocols())
	stop1 := cm1.Start(genesisBlk, sessionCh1)
	defer stop1()

	blk := new(block.Builder).TotalScore(10).ParentID(genesisBlk.Header().ID()).Build()
	ch1.AddBlock(blk, true)

	cm2, srv2, _, genesisBlk := makeAComm(k2, ":50001")
	sessionCh2 := make(chan *p2psrv.Session)
	srv2.SubscribeSession(sessionCh2)
	srv2.Start("thor@111111", cm2.Protocols())
	stop2 := cm2.Start(genesisBlk, sessionCh2)
	defer stop2()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case blk := <-cm2.BlockCh():
				fmt.Printf("[cm2] %v\n", blk)
			}
		}
	}()

	<-time.After(time.Second * 17)
	cancel()
}
