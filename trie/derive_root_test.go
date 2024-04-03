package trie

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

type MockDerivableList struct {
	Elements [][]byte
}

func (m *MockDerivableList) Len() int {
	return len(m.Elements)
}

func (m *MockDerivableList) GetRlp(i int) []byte {
	if i >= len(m.Elements) {
		return nil
	}
	return m.Elements[i]
}

func TestDeriveRoot(t *testing.T) {
	mockList := &MockDerivableList{
		Elements: [][]byte{
			{1, 2, 3, 4},
			{1, 2, 3, 4, 5, 6},
		},
	}

	root := DeriveRoot(mockList)

	assert.Equal(t, "0x154227caf1172839284ce29cd6eaaee115af0993d5a5a4a08d9bb19ed18edae7", root.String())
	assert.NotEqual(t, thor.Bytes32{}, root, "The root hash should not be empty")
}
