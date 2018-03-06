package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/logdb"
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
	optionData := []byte(req.FormValue("options"))
	options := new(logdb.FilterOption)
	if err := json.Unmarshal(optionData, &options); err != nil {
		return err
	}
	logs, err := li.Filter(options)
	if err != nil {
		return httpx.Error("Query logs failed!", 400)
	}
	data, err := json.Marshal(logs)
	if err != nil {
		return err
	}
	w.Write(data)
	return nil
}
