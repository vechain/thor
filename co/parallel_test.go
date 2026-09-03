// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import (
	"testing"
	"time"
)

func TestParallel(t *testing.T) {
	n := 50
	fn := func() {
		time.Sleep(time.Millisecond * 20)
	}

	startTime := time.Now().UnixNano()
	for range n {
		fn()
	}
	t.Log("non-parallel", time.Duration(time.Now().UnixNano()-startTime))

	startTime = time.Now().UnixNano()
	<-Parallel(func(queue chan<- func()) {
		for range n {
			queue <- fn
		}
	})
	t.Log("parallel", time.Duration(time.Now().UnixNano()-startTime))
}
