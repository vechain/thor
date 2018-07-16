// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import (
	"runtime"
	"sync/atomic"
)

var numCPU = runtime.NumCPU()

// Parallel to run a batch of work using as many CPU as it can.
func Parallel(cb func(chan<- func())) <-chan struct{} {
	queue := make(chan func(), numCPU*16)
	defer close(queue)

	done := make(chan struct{})

	nGo := int32(numCPU)
	for i := 0; i < numCPU; i++ {
		go func() {
			for work := range queue {
				work()
			}

			if atomic.AddInt32(&nGo, -1) == 0 {
				close(done)
			}
		}()
	}
	cb(queue)
	return done
}
