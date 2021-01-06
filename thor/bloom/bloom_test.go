// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bloom_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor/bloom"
)

func TestBloom(t *testing.T) {
	const nKey = 100
	const bitsPerKey = 10

	g := &bloom.Generator{}
	for i := 0; i < nKey; i++ {
		g.Add([]byte(fmt.Sprintf("%v", i)))
	}

	f := g.Generate(bitsPerKey, bloom.K(bitsPerKey))

	for i := 0; i < nKey; i++ {
		assert.Equal(t, true, f.Contains([]byte(fmt.Sprintf("%v", i))))
	}

	const nFalseKey = 1000
	nFalse := 0
	for i := 0; i < nFalseKey; i++ {
		if !f.Contains([]byte(fmt.Sprintf("x%v", i))) {
			nFalse++
		}
	}
	t.Log("false test", nFalse, "/", nFalseKey)
}
