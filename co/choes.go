// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import (
	"sync"
)

// Choes runs and manages life-cycle of go routines with and added stop control channel
// Goes + Channel = Choes 🙃
type Choes struct {
	wg       sync.WaitGroup
	stopChan chan struct{}
	once     sync.Once
}

// NewChoes initializes and returns a new Choes instance.
func NewChoes() *Choes {
	return &Choes{
		stopChan: make(chan struct{}),
	}
}

// Go runs f in a go routine. The function f is passed a stop channel
// which it should check to know if it should stop.
func (g *Choes) Go(f func(sc chan struct{})) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		f(g.stopChan)
	}()
}

// Stop signals all go routines to stop by closing the stop channel.
func (g *Choes) Stop() {
	g.once.Do(func() {
		close(g.stopChan)
	})
}

// Wait waits for all go routines started by 'Go' to complete.
func (g *Choes) Wait() {
	g.wg.Wait()
}

// Done returns a channel that is closed when all go routines have finished.
func (g *Choes) Done() chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		g.wg.Wait()
	}()
	return done
}
