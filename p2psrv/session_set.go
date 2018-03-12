package p2psrv

import (
	"math/rand"
	"sync"

	"github.com/ethereum/go-ethereum/p2p/discover"
)

// SessionSet a set of sessions.
type SessionSet struct {
	all map[discover.NodeID]*Session
	mu  sync.Mutex
}

func newSessionSet() *SessionSet {
	return &SessionSet{
		all: make(map[discover.NodeID]*Session),
	}
}

func (ss *SessionSet) add(s *Session) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.all[s.peer.ID()] = s
}

func (ss *SessionSet) remove(s *Session) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.all, s.peer.ID())
}

// Len returns count of sessions.
func (ss *SessionSet) Len() int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return len(ss.all)
}

// All returns a slice of all sessions.
func (ss *SessionSet) All() []*Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	all := make([]*Session, 0, len(ss.all))
	for _, s := range ss.all {
		all = append(all, s)
	}
	return all
}

// Pick pick n sessions randomly.
func (ss *SessionSet) Pick(n int) []*Session {
	all := ss.All()
	perm := rand.Perm(len(all))
	if n > len(all) {
		n = len(all)
	}
	picked := make([]*Session, 0, n)
	for i := 0; i < n; i++ {
		picked = append(picked, all[perm[i]])
	}
	return picked
}
