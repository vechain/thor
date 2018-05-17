// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytes32(t *testing.T) {
	bytes32 := BytesToBytes32([]byte("bytes32"))
	data, _ := json.Marshal(&bytes32)
	assert.Equal(t, "\""+bytes32.String()+"\"", string(data))

	var dec Bytes32
	assert.Nil(t, json.Unmarshal(data, &dec))
	assert.Equal(t, bytes32, dec)
}

func TestAddress(t *testing.T) {
	addr := BytesToAddress([]byte("addr"))
	data, _ := json.Marshal(&addr)
	assert.Equal(t, "\""+addr.String()+"\"", string(data))

	var dec Address
	assert.Nil(t, json.Unmarshal(data, &dec))
	assert.Equal(t, addr, dec)
}
