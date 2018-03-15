package co

import (
	"sync"
)

// Goes to run and manage life-cycle of go routines.
type Goes struct {
	wg sync.WaitGroup
}

// Go run f in go routine.
func (g *Goes) Go(f func()) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		f()
	}()
}

// Wait wait for all go routines started by 'Go' done.
func (g *Goes) Wait() {
	g.wg.Wait()
}

// Done return the done channel for exiting of all go routines.
func (g *Goes) Done() chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		g.wg.Wait()
	}()
	return done
}
