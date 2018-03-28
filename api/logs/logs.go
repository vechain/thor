package logs

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
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
	var options *logdb.FilterOption
	if len(res) != 0 {
		if err := json.Unmarshal(res, &options); err != nil {
			return utils.HTTPError(err, http.StatusBadRequest)
		}
	}
	logs, err := l.filter(options)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, logs)
}

func (l *Logs) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(l.handleFilterLogs))
}
