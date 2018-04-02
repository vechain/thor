package logs

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/logdb"
)

type Logs struct {
	logDB *logdb.LogDB
}

func New(logDB *logdb.LogDB) *Logs {
	return &Logs{
		logDB,
	}
}

//Filter query logs with option
func (l *Logs) filter(option *logdb.FilterOption) ([]Log, error) {
	logs, err := l.logDB.Filter(option)
	if err != nil {
		return nil, err
	}
	lgs := make([]Log, len(logs))
	for i, log := range logs {
		lgs[i] = convertLog(log)
	}
	return lgs, nil
}

func (l *Logs) handleFilterLogs(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	req.Body.Close()
	var f *Filters
	if len(res) != 0 {
		if err := json.Unmarshal(res, &f); err != nil {
			return utils.HTTPError(err, http.StatusBadRequest)
		}
	}
	fromBlock, err := l.parseFromBlock(req.URL.Query().Get("fromBlock"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "fromBlock"), http.StatusBadRequest)
	}
	toBlock, err := l.parseToBlock(req.URL.Query().Get("toBlock"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "toBlock"), http.StatusBadRequest)
	}
	offset, err := l.parseOffset(req.URL.Query().Get("offset"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "offset"), http.StatusBadRequest)
	}
	limit, err := l.parseLimit(req.URL.Query().Get("limit"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "limit"), http.StatusBadRequest)
	}
	options := &logdb.FilterOption{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Address:   f.Address,
		TopicSet:  f.TopicSet,
		Offset:    offset,
		Limit:     limit,
	}
	logs, err := l.filter(options)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, logs)
}

func (l *Logs) parseOffset(offset string) (uint64, error) {
	if offset == "" {
		return math.MaxUint64, nil
	}
	n, err := strconv.ParseUint(offset, 0, 0)
	if err != nil {
		return math.MaxUint64, err
	}
	return uint64(n), nil
}

func (l *Logs) parseLimit(limit string) (uint32, error) {
	if limit == "" {
		return math.MaxUint32, nil
	}
	n, err := strconv.ParseUint(limit, 0, 0)
	if err != nil {
		return math.MaxUint32, err
	}
	if n > math.MaxUint32 {
		return math.MaxUint32, errors.New("block number exceeded")
	}
	return uint32(n), nil
}

func (l *Logs) parseFromBlock(blkNum string) (uint32, error) {
	if blkNum == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(blkNum, 0, 0)
	if err != nil {
		return math.MaxUint32, err
	}
	if n > math.MaxUint32 {
		return math.MaxUint32, errors.New("block number exceeded")
	}
	return uint32(n), nil
}

func (l *Logs) parseToBlock(blkNum string) (uint32, error) {
	if blkNum == "" {
		return math.MaxUint32, nil
	}
	n, err := strconv.ParseUint(blkNum, 0, 0)
	if err != nil {
		return math.MaxUint32, err
	}
	if n > math.MaxUint32 {
		return math.MaxUint32, errors.New("block number exceeded")
	}
	return uint32(n), nil
}

func (l *Logs) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(l.handleFilterLogs))
}
