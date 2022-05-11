package debug

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers/logger"
)

type TracerOption struct {
	Name   string         `json:"name"`
	Target string         `json:"target"`
	Config *logger.Config `json:"config"`
}

type StorageRangeOption struct {
	Address   thor.Address
	KeyStart  string
	MaxResult int
	Target    string
}

type StorageRangeResult struct {
	Storage StorageMap    `json:"storage"`
	NextKey *thor.Bytes32 `json:"nextKey"` // nil if Storage includes the last key in the trie.
}

type StorageMap map[string]StorageEntry

type StorageEntry struct {
	Key   *thor.Bytes32 `json:"key"`
	Value *thor.Bytes32 `json:"value"`
}
