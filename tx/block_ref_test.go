// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"crypto/rand"
	"testing"

	"github.com/vechain/thor/v2/thor"

	"github.com/stretchr/testify/assert"
)

func TestBlockRef(t *testing.T) {
	assert.Equal(t, uint32(0), BlockRef{}.Number())

	assert.Equal(t, BlockRef{0, 0, 0, 0xff, 0, 0, 0, 0}, NewBlockRef(0xff))

	var bid thor.Bytes32
	_, err := rand.Read(bid[:])
	assert.NoError(t, err)

	br := NewBlockRefFromID(bid)
	assert.Equal(t, bid[:8], br[:])
}
