// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx_test

import (
	"math/rand"
	"testing"

	"github.com/vechain/thor/thor"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/tx"
)

func TestBlockRef(t *testing.T) {
	assert.Equal(t, uint32(0), tx.BlockRef{}.Number())

	assert.Equal(t, tx.BlockRef{0, 0, 0, 0xff, 0, 0, 0, 0}, tx.NewBlockRef(0xff))

	var bid thor.Bytes32
	rand.Read(bid[:])

	br := tx.NewBlockRefFromID(bid)
	assert.Equal(t, bid[:8], br[:])
}
