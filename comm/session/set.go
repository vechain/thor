package session

import (
	"math/rand"
	"sync"

	"github.com/ethereum/go-ethereum/p2p/discover"
)

// Slice slice of sessions
type Slice []*Session

// Filter filter out sub set of sessions that satisfies the given condition.
func (ss Slice) Filter(cond func(*Session) bool) Slice {
	ret := make(Slice, 0, len(ss))
	for _, s := range ss {
		if cond(s) {
			ret = append(ret, s)
		}
	}
	return ret
}

// Set manages a set of sessions, which mapped by NodeID.
type Set struct {
	m    map[discover.NodeID]*Session
	lock sync.Mutex
}

// NewSet create a session Set instance.
func NewSet() *Set {
	return &Set{
		m: make(map[discover.NodeID]*Session),
	}
}

// Add add a new session.
func (ss *Set) Add(session *Session) {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	ss.m[session.peer.ID()] = session
}

// Find find session for given nodeID.
func (ss *Set) Find(nodeID discover.NodeID) *Session {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	return ss.m[nodeID]
}

// Remove removes session for given nodeID.
func (ss *Set) Remove(nodeID discover.NodeID) *Session {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	if session, ok := ss.m[nodeID]; ok {
		delete(ss.m, nodeID)
		return session
	}
	return nil
}

// Slice dumps all sessions into a slice.
// The dumped slice is a random permutation.
func (ss *Set) Slice() Slice {
	ss.lock.Lock()
	defer ss.lock.Unlock()

	ret := make(Slice, len(ss.m))
	perm := rand.Perm(len(ss.m))
	i := 0
	for _, s := range ss.m {
		// randomly
		ret[perm[i]] = s
		i++
	}
	return ret
}
