package logs

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
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
func (l *Logs) filter(logFilter *LogFilter) ([]FilteredLog, error) {
	lf := convertLogFilter(logFilter)
	logs, err := l.logDB.Filter(lf)
	if err != nil {
		return nil, err
	}
	lgs := make([]FilteredLog, len(logs))
	for i, log := range logs {
		lgs[i] = convertLog(log)
	}
	return lgs, nil
}

func (l *Logs) handleFilterLogs(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	logFilter := new(LogFilter)
	if len(res) != 0 {
		if err := json.Unmarshal(res, &logFilter); err != nil {
			return err
		}
	}
	query := req.URL.Query()
	if query.Get("address") != "" {
		addr, err := thor.ParseAddress(query.Get("address"))
		if err != nil {
			return utils.BadRequest(err, "address")
		}
		logFilter.Address = &addr
	}
	order := query.Get("order")
	if order != string(logdb.DESC) {
		logFilter.Order = logdb.ASC
	} else {
		logFilter.Order = logdb.DESC
	}
	logs, err := l.filter(logFilter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, logs)
}

func (l *Logs) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(l.handleFilterLogs))
}
