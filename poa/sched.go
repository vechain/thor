package poa

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/thor"
)

// Scheduler to schedule the time when a proposer to produce a block.
type Scheduler struct {
	proposers         []Proposer
	parentBlockNumber uint32
	parentBlockTime   uint64
}

// NewScheduler create a Scheduler object.
func NewScheduler(
	proposers []Proposer,
	parentBlockNumber uint32,
	parentBlockTime uint64) *Scheduler {
	return &Scheduler{
		append([]Proposer(nil), proposers...),
		parentBlockNumber,
		parentBlockTime,
	}
}

// Schedule to determine time of the proposer to produce a block, according to `nowTime`.
// If the proposer is not listed, an error returned.
// `newBlockTime` is promised to be >= nowTime
func (s *Scheduler) Schedule(addr thor.Address, nowTime uint64) (
	newBlockTime uint64,
	updates []Proposer,
	err error,
) {
	// gather online proposers including addr
	// and check if addr is listed
	onlines := make([]Proposer, 0, len(s.proposers))
	authorized := false
	for _, p := range s.proposers {
		if p.Address == addr {
			onlines = append(onlines, p)
			authorized = true
		} else if p.IsOnline() {
			onlines = append(onlines, p)
		}
	}
	if !authorized {
		return 0, nil, errors.New("unauthorized block proposer")
	}

	const T = thor.BlockInterval

	// initialize newBlockTime
	if nowTime > s.parentBlockTime {
		// ensure T aligned
		newBlockTime += s.parentBlockTime + (nowTime-s.parentBlockTime)/T*T
	} else {
		newBlockTime = s.parentBlockTime + T
	}

	updateMap := make(proposerMap)
	for {
		if newBlockTime >= nowTime {
			// deterministic pseudo-random process
			index := s.rand(newBlockTime) % uint32(len(onlines))
			p := onlines[index]
			if p.Address == addr {
				if !p.IsOnline() {
					// mark addr online if not
					p.SetOnline(true)
					updateMap[p.Address] = p
				}
				break
			}
		}
		newBlockTime += T
	}

	// traverse back at most len(s.proposers) periods of T
	// to collect offline proposers.
	for i := range s.proposers {
		t := newBlockTime - uint64(i+1)*T
		if t <= s.parentBlockTime {
			break
		}

		index := s.rand(t) % uint32(len(onlines))
		p := onlines[index]
		if p.Address != addr {
			if p.IsOnline() {
				// mark offline if not
				p.SetOnline(false)
				updateMap[p.Address] = p
			}
		}
	}

	updates = updateMap.toSlice()
	return
}

// rand deterministic pseudo-random function.
// H(B, t)[:4]
func (s *Scheduler) rand(t uint64) uint32 {
	var bin [12]byte
	binary.BigEndian.PutUint32(bin[:], s.parentBlockNumber)
	binary.BigEndian.PutUint64(bin[4:], t)
	sum := sha256.Sum256(bin[:])
	return binary.BigEndian.Uint32(sum[:])
}

// CalculateScore calculates score, which is the count of currently online proposers.
func CalculateScore(all []Proposer, updates []Proposer) uint64 {
	onlines := make(map[thor.Address]interface{})
	for _, p := range all {
		if p.IsOnline() {
			onlines[p.Address] = nil
		}
	}

	for _, p := range updates {
		if p.IsOnline() {
			onlines[p.Address] = nil
		} else {
			delete(onlines, p.Address)
		}
	}
	return uint64(len(onlines))
}

type proposerMap map[thor.Address]Proposer

func (pm proposerMap) toSlice() []Proposer {
	slice := make([]Proposer, 0, len(pm))
	for _, p := range pm {
		slice = append(slice, p)
	}
	return slice
}
