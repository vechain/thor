package vmlog

import "github.com/ethereum/go-ethereum/core/types"

type VMlog struct {
}

// New return a new VMlog point.
func New() *VMlog {
	return &VMlog{}
}

func (vl *VMlog) AddLog(*types.Log) {
}
