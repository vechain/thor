package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/logdb"
	"io/ioutil"
	"net/http"
)

//LogHTTPPathPrefix http path prefix
const LogHTTPPathPrefix = "/logs"

//NewLogHTTPRouter add path to router
func NewLogHTTPRouter(router *mux.Router, li *LogInterface) {
	sub := router.PathPrefix(LogHTTPPathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(httpx.WrapHandlerFunc(li.handleFilterLogs))

}

func (li *LogInterface) handleFilterLogs(w http.ResponseWriter, req *http.Request) error {
	r, err := ioutil.ReadAll(req.Body)
	req.Body.Close()
	var options *logdb.FilterOption
	if len(r) != 0 {
		if err := json.Unmarshal(r, &options); err != nil {
			return err
		}
	}
	logs, err := li.Filter(options)
	if err != nil {
		return httpx.Error("Query logs failed!", 400)
	}
	return httpx.ResponseJSON(w, logs)
}
