// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm_test

import (
	"testing"

	"slices"

	"github.com/stretchr/testify/assert"
)

func TestLoopVar(t *testing.T) {
	cases := []uint{1, 2, 3, 4}

	ch := make(chan uint)

	go func() {
		for _, c := range cases {
			go func() {
				ch <- c
			}()
		}
	}()

	ret := make([]uint, 0, len(cases))
	for range cases {
		v := <-ch
		ret = append(ret, v)
	}
	slices.Sort(ret)
	// before go1.22 the result is [4, 4, 4, 4]
	// after go1.22 the result is [1, 2, 3, 4]

	assert.Equal(t, cases, ret)
}
