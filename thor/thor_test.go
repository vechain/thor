package thor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash(t *testing.T) {
	hash := BytesToHash([]byte("hash"))
	data, _ := json.Marshal(&hash)
	assert.Equal(t, "\""+hash.String()+"\"", string(data))

	var dec Hash
	assert.Nil(t, json.Unmarshal(data, &dec))
	assert.Equal(t, hash, dec)
}

func TestAddress(t *testing.T) {
	addr := BytesToAddress([]byte("addr"))
	data, _ := json.Marshal(&addr)
	assert.Equal(t, "\""+addr.String()+"\"", string(data))

	var dec Address
	assert.Nil(t, json.Unmarshal(data, &dec))
	assert.Equal(t, addr, dec)
}
