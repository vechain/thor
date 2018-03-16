package comm

import (
	"sync"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/p2psrv"
)

type sessions []*p2psrv.Session

func (ss sessions) filter(cond func(s *p2psrv.Session) bool) sessions {
	ret := make(sessions, 0, len(ss))
	for _, s := range ss {
		if cond(s) {
			ret = append(ret, s)
		}
	}

	return ret
}

type sessionSet struct {
	sync.Mutex
	m map[discover.NodeID]*p2psrv.Session
}

func (sset sessionSet) remove(key discover.NodeID) {
	sset.Lock()
	defer sset.Unlock()

	delete(sset.m, key)
}

func (sset sessionSet) add(key discover.NodeID, session *p2psrv.Session) {
	sset.Lock()
	defer sset.Unlock()

	sset.m[key] = session
}

func (sset sessionSet) getSessions() sessions {
	sset.Lock()
	defer sset.Unlock()

	ret := make(sessions, 0, len(sset.m))
	for _, s := range sset.m {
		ret = append(ret, s)
	}

	return ret
}
