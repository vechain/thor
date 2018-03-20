package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/api/utils/types"
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
	var options types.FilterOption
	fmt.Println(optionData)
	if len(optionData) != 0 {
		if err := json.Unmarshal(optionData, &options); err != nil {
			fmt.Println(1, err)
			return err
		}
	}
	logs, err := li.Filter(options)
	if err != nil {
		fmt.Println(2, err)
		return httpx.Error("Query logs failed!", 400)
	}
	fmt.Println(logs)
	data, err := json.Marshal(logs)
	if err != nil {
		fmt.Println(3, err)
		return err
	}

	w.Write(data)
	return nil
}
