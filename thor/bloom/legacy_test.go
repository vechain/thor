// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bloom_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor/bloom"
)

func TestLegacyBloom(t *testing.T) {
	itemCount := 100

	bloom := bloom.NewLegacyBloom(bloom.LegacyEstimateBloomK(itemCount))

	for i := range itemCount {
		bloom.Add(fmt.Appendf(nil, "%v", i))
	}

	for i := range itemCount {
		assert.Equal(t, true, bloom.Test(fmt.Appendf(nil, "%v", i)))
	}
}
