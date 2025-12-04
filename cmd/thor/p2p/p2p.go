// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/p2p/discover"
	"github.com/vechain/thor/v2/p2p/nat"

	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/p2psrv"
)

type P2P struct {
	comm           *comm.Communicator
	p2pSrv         *p2psrv.Server
	peersCachePath string
	enode          string
}

func New(
	communicator *comm.Communicator,
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
) *P2P {
	// known peers will be loaded/stored from/in this file
	peersCachePath := filepath.Join(instanceDir, "peers.cache")

	// default option setting
	// no known nodes for p2p connection
	// use the hardcoded fallbackDiscoveryNodes for discovery only
	opts := &p2psrv.Options{
		Name:                common.MakeName("thor", version),
		PrivateKey:          privateKey,
		MaxPeers:            maxPeers,
		ListenAddr:          listenAddr,
		DiscoveryNodes:      fallbackDiscoveryNodes,
		RemoteDiscoveryList: remoteDiscoveryNodesList,
		NAT:                 nat,
	}

	// allowed peers flag will only allow p2psrv to connect to the designated peers
	if len(allowedPeers) > 0 {
		opts.NoDiscovery = true // disable discovery
		opts.DiscoveryNodes = nil
		opts.KnownNodes = allowedPeers
	} else {
		// bootstrap nodes flag will overwrite the default discovery nodes and also disable remote discovery
		if len(bootstrapNodes) > 0 {
			opts.RemoteDiscoveryList = ""        // disable remote discovery
			opts.DiscoveryNodes = bootstrapNodes // overwrite the default discovery nodes
			opts.KnownNodes = bootstrapNodes     // supplied bootstrap nodes can potentially be p2p node, add to the known nodes
		}

		// cached peers will be appended to existing or flag-set bootstrap nodes
		if len(cachedPeers) > 0 {
			opts.KnownNodes = dedupNodeSlice(opts.KnownNodes, cachedPeers)
		}
	}

	return &P2P{
		comm:           communicator,
		p2pSrv:         p2psrv.New(opts),
		peersCachePath: peersCachePath,
		enode:          fmt.Sprintf("enode://%x@[extip]:%v", discover.PubkeyID(&privateKey.PublicKey).Bytes(), listenPort),
	}
}

func (p *P2P) Start() error {
	log.Info("starting P2P networking")
	if err := p.p2pSrv.Start(p.comm.Protocols(), p.comm.DiscTopic()); err != nil {
		return fmt.Errorf("start P2P server: %w", err)
	}
	p.comm.Start()
	return nil
}

func (p *P2P) Stop() {
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
	if err := os.WriteFile(p.peersCachePath, data, 0o600); err != nil {
		log.Warn("failed to write peers cache", "err", err)
	}
}

func (p *P2P) Communicator() *comm.Communicator {
	return p.comm
}

func (p *P2P) Enode() string {
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
