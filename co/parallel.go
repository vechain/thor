// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import (
	"runtime"
)

var numCPU = runtime.NumCPU()

// Enqueue function to enqueue parallel works.
type Enqueue func(work func())

// Parallel to run a batch of work using as many CPU as it can.
func Parallel(cb func(Enqueue)) {
	if numCPU < 2 {
		cb(func(work func()) {
			work()
		})
	}

	var goes Goes
	defer goes.Wait()
	ch := make(chan func(), numCPU*2)
	defer close(ch)
	for i := 0; i < numCPU; i++ {
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
