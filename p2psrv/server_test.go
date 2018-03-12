package p2psrv_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
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

func handleRequest(session *p2psrv.Session, msg *p2p.Msg) (resp interface{}, err error) {
	var req string
	if err := msg.Decode(&req); err != nil {
		panic(err)
	}
	fmt.Println("req: ", req)
	return "bar", nil
}

func TestServer(t *testing.T) {
	srv1 := p2psrv.New(
		&p2psrv.Options{
			PrivateKey: mustHexToECDSA(k1),
			MaxPeers:   25,
			ListenAddr: ":40001",

			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})

	srv2 := p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     mustHexToECDSA(k2),
			MaxPeers:       25,
			ListenAddr:     ":50001",
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})
	proto := &p2psrv.Protocol{
		Name:          "MyProtocol",
		Version:       1,
		Length:        1,
		HandleRequest: handleRequest,
	}

	srv1.Start("thor@111111", []*p2psrv.Protocol{proto})
	defer srv1.Stop()
	srv2.Start("thor@111111", []*p2psrv.Protocol{proto})
	defer srv2.Stop()

	go func() {
		for {
			all := srv1.SessionSet().All()
			if len(all) > 0 {
				var resp string
				if err := all[0].Request(context.Background(), 0, "foo", &resp); err != nil {
					panic(err)
				}
				fmt.Println("resp:", resp)
				break
			}
			<-time.After(time.Millisecond * 100)
		}
	}()

	fmt.Println(srv1.Self())
	fmt.Println(srv2.Self())
	<-time.After(time.Second * 4)
}
