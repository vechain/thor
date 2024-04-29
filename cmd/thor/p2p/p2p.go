package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/p2psrv"
)

type ThorP2P struct {
	comm           *comm.Communicator
	p2pSrv         *p2psrv.Server
	peersCachePath string
	enode          string
}

func New(
	p2pCom *comm.Communicator,
	privateKey *ecdsa.PrivateKey,
	instanceDir string,
	nat nat.Interface,
	version string,
	maxPeers int,
	listenPort int,
	listenAddr string,
	allowedPeers []*discover.Node,
	cachedPeers []*discover.Node,
	bootstrapNodes []*discover.Node,
) *ThorP2P {
	// known peers will be loaded/stored from/in this file
	peersCachePath := filepath.Join(instanceDir, "peers.cache")

	// default option setting
	opts := &p2psrv.Options{
		Name:            common.MakeName("thor", version),
		PrivateKey:      privateKey,
		MaxPeers:        maxPeers,
		ListenAddr:      listenAddr,
		KnownNodes:      fallbackBootstrapNodes,
		RemoteBootstrap: remoteBootstrapList,
		NAT:             nat,
	}

	// allowed peers flag will only allow p2psrv to connect to the designated peers
	if len(allowedPeers) > 0 {
		opts.NoDiscovery = true // disable discovery
		opts.KnownNodes = allowedPeers
	} else {
		// boot nodes flag will overwrite the default bootstrap nodes and also disable remote bootstrap
		if len(bootstrapNodes) > 0 {
			opts.RemoteBootstrap = "" // disable remote bootstrap
			opts.KnownNodes = bootstrapNodes
		}

		// cached peers will be appended to existing or set bootnodes
		if len(cachedPeers) > 0 {
			opts.KnownNodes = dedupNodeSlice(opts.KnownNodes, cachedPeers)
		}
	}

	return &ThorP2P{
		comm:           p2pCom,
		p2pSrv:         p2psrv.New(opts),
		peersCachePath: peersCachePath,
		enode:          fmt.Sprintf("enode://%x@[extip]:%v", discover.PubkeyID(&privateKey.PublicKey).Bytes(), listenPort),
	}
}

func (p *ThorP2P) Start() error {
	log.Info("starting P2P networking")
	if err := p.p2pSrv.Start(p.comm.Protocols(), p.comm.DiscTopic()); err != nil {
		return errors.Wrap(err, "start P2P server")
	}
	p.comm.Start()
	return nil
}

func (p *ThorP2P) Stop() {
	log.Info("stopping communicator...")
	p.comm.Stop()

	log.Info("stopping P2P server...")
	p.p2pSrv.Stop()

	log.Info("saving peers cache...")
	nodes := p.p2pSrv.KnownNodes()
	data, err := rlp.EncodeToBytes(nodes)
	if err != nil {
		log.Warn("failed to encode cached peers", "err", err)
		return
	}
	if err := os.WriteFile(p.peersCachePath, data, 0600); err != nil {
		log.Warn("failed to write peers cache", "err", err)
	}
}

func (p *ThorP2P) Communicator() *comm.Communicator {
	return p.comm
}

func (p *ThorP2P) Enode() string {
	return p.enode
}

func dedupNodeSlice(slice1, slice2 p2psrv.Nodes) p2psrv.Nodes {
	foundMap := map[string]bool{}
	var dedupedSlice p2psrv.Nodes

	for _, item := range append(slice1, slice2...) {
		if _, ok := foundMap[item.ID.String()]; ok {
			continue
		}
		foundMap[item.ID.String()] = true
		dedupedSlice = append(dedupedSlice, item)
	}

	return dedupedSlice
}
