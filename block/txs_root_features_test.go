// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
)

func TestTrf(t *testing.T) {
	var b32 thor.Bytes32
	rand.Read(b32[:])

	obj := txsRootFeatures{
		Root: b32,
	}

	data1, err := rlp.EncodeToBytes(&obj)
	assert.Nil(t, err)

	data2, err := rlp.EncodeToBytes(b32)
	assert.Nil(t, err)

	assert.EqualValues(t, data2, data1)

	var d thor.Bytes32
	assert.Nil(t, rlp.DecodeBytes(data1, &d))
	assert.Equal(t, b32, d)

	////
	obj.Features = 1

	data1, err = rlp.EncodeToBytes(&obj)
	assert.Nil(t, err)

	var obj2 txsRootFeatures
	assert.Nil(t, rlp.DecodeBytes(data1, &obj2))

	assert.EqualValues(t, obj, obj2)
}
