package api

import (
	"github.com/vechain/thor/api/types"
	"github.com/vechain/thor/logdb"
)

//LogInterface for query logs
type LogInterface struct {
	ldb *logdb.LogDB
}

//NewLogInterface new LogInterface
func NewLogInterface(ldb *logdb.LogDB) *LogInterface {
	return &LogInterface{
		ldb,
	}
}

//Filter query logs with option
func (li *LogInterface) Filter(option *logdb.FilterOption) ([]types.Log, error) {
	logs, err := li.ldb.Filter(option)
	if err != nil {
		return nil, err
	}
	lgs := make([]types.Log, len(logs))
	for i, log := range logs {
		lgs[i] = types.ConvertLog(log)
	}
	return lgs, nil
}
