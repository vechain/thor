package p2psrv

import (
	"math/rand"
)

// Sessions session slice.
type Sessions []*Session

// Pick pick n sessions randomly.
func (ss Sessions) Pick(n int) Sessions {
	perm := rand.Perm(len(ss))
	if n > len(ss) {
		n = len(ss)
	}
	picked := make(Sessions, 0, n)
	for i := 0; i < n; i++ {
		picked = append(picked, ss[perm[i]])
	}
	return picked
}

// Filter filter out sessions satisfy cond.
func (ss Sessions) Filter(cond func(s *Session) bool) Sessions {
	ret := make(Sessions, 0, len(ss))
	for _, s := range ss {
		if cond(s) {
			ret = append(ret, s)
		}
	}
	return ret
}
