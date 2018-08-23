// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import "sync"

// Signal a rendezvous point for goroutines waiting for or announcing the occurrence of an event.
// It's more friendly than sync.Cond, since it's channel base. That means you can do channel selection
// to wait for an event.
type Signal struct {
	l  sync.Mutex
	ch chan bool
}

func (s *Signal) init() {
	if s.ch == nil {
		s.ch = make(chan bool, 1)
	}
}

// Signal wakes one goroutine that is waiting on s.
func (s *Signal) Signal() {
	s.l.Lock()

	s.init()
	select {
	case s.ch <- true:
	default:
	}

	s.l.Unlock()
}

// Broadcast wakes all goroutines that are waiting on s.
func (s *Signal) Broadcast() {
	s.l.Lock()

	s.init()
	close(s.ch)
	s.ch = make(chan bool, 1)

	s.l.Unlock()
}

// Waiter returns a function which for acquiring channel to wait for.
// Value read from channel indicates signal or broadcast. true for signal, otherwise broadcast.
func (s *Signal) Waiter() func() <-chan bool {
	s.l.Lock()

	s.init()
	ref := s.ch

	s.l.Unlock()

	return func() (ch <-chan bool) {
		ch = ref

		s.l.Lock()
		ref = s.ch
		s.l.Unlock()

		return
	}
}
