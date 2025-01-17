// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheStats(t *testing.T) {
	cs := &Stats{}
	cs.Hit()
	cs.Miss()
	_, hit, miss := cs.Stats()

	assert.Equal(t, int64(1), hit)
	assert.Equal(t, int64(1), miss)

	changed, _, _ := cs.Stats()
	assert.False(t, changed)

	cs.Hit()
	cs.Miss()
	assert.Equal(t, int64(3), cs.Hit())

	changed, hit, miss = cs.Stats()

	assert.Equal(t, int64(3), hit)
	assert.Equal(t, int64(2), miss)
	assert.True(t, changed)
}
