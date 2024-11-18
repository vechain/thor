package muxdb

import "github.com/vechain/thor/v2/trie"

type emptyCache struct{}

// AddNodeBlob does nothing.
func (e *emptyCache) AddNodeBlob(_ *[]byte, _ string, _ []byte, _ trie.Version, _ []byte, _ bool) {
}

// GetNodeBlob always returns nil.
func (e *emptyCache) GetNodeBlob(_ *[]byte, _ string, _ []byte, _ trie.Version, _ bool) []byte {
	return nil
}

// AddRootNode does nothing.
func (e *emptyCache) AddRootNode(_ string, _ trie.Node) {}

// GetRootNode always returns nil.
func (e *emptyCache) GetRootNode(_ string, _ trie.Version) trie.Node {
	return nil
}
