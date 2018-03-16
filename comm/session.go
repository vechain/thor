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
	lock sync.Mutex
	m    map[discover.NodeID]*p2psrv.Session
}

func (ss *sessionSet) remove(key discover.NodeID) {
	ss.lock.Lock()
	defer ss.lock.Unlock()

	delete(ss.m, key)
}

func (ss *sessionSet) add(key discover.NodeID, session *p2psrv.Session) {
	ss.lock.Lock()
	defer ss.lock.Unlock()

	ss.m[key] = session
}

func (ss *sessionSet) slice() sessions {
	ss.lock.Lock()
	defer ss.lock.Unlock()

	ret := make(sessions, 0, len(ss.m))
	for _, s := range ss.m {
		ret = append(ret, s)
	}

	return ret
}
