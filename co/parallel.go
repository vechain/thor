package co

import (
	"runtime"
)

// Enqueue function to enqueue parallel works.
type Enqueue func(work func())

// Parallel to run a batch of work using as many CPU as it can.
func Parallel(cb func(Enqueue)) {
	var goes Goes
	defer goes.Wait()
	ch := make(chan func(), runtime.NumCPU()*2)
	defer close(ch)
	for i := 0; i < runtime.NumCPU(); i++ {
		goes.Go(func() {
			for {
				select {
				case work := <-ch:
					if work == nil {
						return
					}
					work()
				}
			}
		})
	}
	cb(func(work func()) { ch <- work })
}
