// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	lru "github.com/hashicorp/golang-lru"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/p2psrv/rpc"
	"github.com/vechain/thor/thor"
)

const (
	maxKnownTxs           = 32768 // Maximum transactions IDs to keep in the known list (prevent DOS)
	maxKnownBlocks        = 1024  // Maximum block IDs to keep in the known list (prevent DOS)
	maxKnownProposals     = 1024  // TBD: Maximum block proposals to keep in the know list(prevent DOS)
	maxKnownApprovals     = 1024  // TBD: Maximum block approvals to keep in the know list(prevent DOS)
	knownTxMarkExpiration = 10    // Time in seconds to expire known tx mark
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Peer extends p2p.Peer with RPC integrated.
type Peer struct {
	*p2p.Peer
	*rpc.RPC
	logger log15.Logger

	createdTime    mclock.AbsTime
	knownTxs       *lru.Cache
	knownBlocks    *lru.Cache
	knownProposals *lru.Cache
	knownApprovals *lru.Cache
	head           struct {
		sync.Mutex
		id         thor.Bytes32
		totalScore uint64
	}
}

func newPeer(peer *p2p.Peer, rw p2p.MsgReadWriter) *Peer {
	dir := "outbound"
	if peer.Inbound() {
		dir = "inbound"
	}
	ctx := []interface{}{
		"peer", peer,
		"dir", dir,
	}
	knownTxs, _ := lru.New(maxKnownTxs)
	knownBlocks, _ := lru.New(maxKnownBlocks)
	knownProposals, _ := lru.New(maxKnownProposals)
	knownApprovals, _ := lru.New(maxKnownApprovals)
	return &Peer{
		Peer:           peer,
		RPC:            rpc.New(peer, rw),
		logger:         log.New(ctx...),
		createdTime:    mclock.Now(),
		knownTxs:       knownTxs,
		knownBlocks:    knownBlocks,
		knownProposals: knownProposals,
		knownApprovals: knownApprovals,
	}
}

// Head returns head block ID and total score.
func (p *Peer) Head() (id thor.Bytes32, totalScore uint64) {
	p.head.Lock()
	defer p.head.Unlock()
	return p.head.id, p.head.totalScore
}

// UpdateHead update ID and total score of head block.
func (p *Peer) UpdateHead(id thor.Bytes32, totalScore uint64) {
	p.head.Lock()
	defer p.head.Unlock()
	if totalScore > p.head.totalScore {
		p.head.id, p.head.totalScore = id, totalScore
	}
}

// MarkTransaction marks a transaction to known.
func (p *Peer) MarkTransaction(hash thor.Bytes32) {
	p.knownTxs.Add(hash, time.Now().Unix())
}

// MarkBlock marks a block to known.
func (p *Peer) MarkBlock(id thor.Bytes32) {
	p.knownBlocks.Add(id, struct{}{})
}

// MarkProposal marks a proposal to known.
func (p *Peer) MarkProposal(hash thor.Bytes32) {
	p.knownProposals.Add(hash, struct{}{})
}

// MarkApproval marks an approval to known.
func (p *Peer) MarkApproval(hash thor.Bytes32) {
	p.knownApprovals.Add(hash, struct{}{})
}

// IsTransactionKnown returns if the transaction is known.
func (p *Peer) IsTransactionKnown(hash thor.Bytes32) bool {
	ts, ok := p.knownTxs.Get(hash)
	if !ok {
		return false
	}
	return ts.(int64)+knownTxMarkExpiration > time.Now().Unix()
}

// IsBlockKnown returns if the block is known.
func (p *Peer) IsBlockKnown(id thor.Bytes32) bool {
	return p.knownBlocks.Contains(id)
}

// IsProposalKnown returns if the proposal is known.
func (p *Peer) IsProposalKnown(hash thor.Bytes32) bool {
	return p.knownProposals.Contains(hash)
}

// IsApprovalKnown returns if the approval is known.
func (p *Peer) IsApprovalKnown(hash thor.Bytes32) bool {
	return p.knownApprovals.Contains(hash)
}

// Duration returns duration of connection.
func (p *Peer) Duration() mclock.AbsTime {
	return mclock.Now() - p.createdTime
}

// Peers slice of peers
type Peers []*Peer

// Filter filter out sub set of peers that satisfies the given condition.
func (ps Peers) Filter(cond func(*Peer) bool) Peers {
	ret := make(Peers, 0, len(ps))
	for _, peer := range ps {
		if cond(peer) {
			ret = append(ret, peer)
		}
	}
	return ret
}

// Find find one peer that satisfies the given condition.
func (ps Peers) Find(cond func(*Peer) bool) *Peer {
	for _, peer := range ps {
		if cond(peer) {
			return peer
		}
	}
	return nil
}

// PeerSet manages a set of peers, which mapped by NodeID.
type PeerSet struct {
	m    map[discover.NodeID]*Peer
	lock sync.Mutex
}

// NewSet create a peer set instance.
func newPeerSet() *PeerSet {
	return &PeerSet{
		m: make(map[discover.NodeID]*Peer),
	}
}

// Add add a new peer.
func (ps *PeerSet) Add(peer *Peer) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	ps.m[peer.ID()] = peer
}

// Find find peer for given nodeID.
func (ps *PeerSet) Find(nodeID discover.NodeID) *Peer {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	return ps.m[nodeID]
}

// Remove removes peer for given nodeID.
func (ps *PeerSet) Remove(nodeID discover.NodeID) *Peer {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	if peer, ok := ps.m[nodeID]; ok {
		delete(ps.m, nodeID)
		return peer
	}
	return nil
}

// Slice dumps all peers into a slice.
// The dumped slice is a random permutation.
func (ps *PeerSet) Slice() Peers {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	ret := make(Peers, len(ps.m))
	perm := rand.Perm(len(ps.m))
	i := 0
	for _, s := range ps.m {
		// randomly
		ret[perm[i]] = s
		i++
	}
	return ret
}

// Len returns length of set.
func (ps *PeerSet) Len() int {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	return len(ps.m)
}
