package vmlog

import (
	"github.com/ethereum/go-ethereum/core/types"
)

type VMlog struct {
	logs []*types.Log
}

// New return a new VMlog point.
func New() *VMlog {
	return &VMlog{}
}

func (vl *VMlog) AddLog(log *types.Log) {
	vl.logs = append(vl.logs, log)
}

func (vl *VMlog) GetLogs() []*types.Log {
	return vl.logs
}
