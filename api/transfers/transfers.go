package transfers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/vechain/thor/transferdb"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/logdb"
)

type Transfers struct {
	transferDB *transferdb.TransferDB
}

func New(transferDB *transferdb.TransferDB) *Transfers {
	return &Transfers{
		transferDB,
	}
}

//Filter query logs with option
func (t *Transfers) filter(transferFilter *transferdb.TransferFilter) ([]*FilteredTransfer, error) {
	transfers, err := t.transferDB.Filter(transferFilter)
	if err != nil {
		return nil, err
	}
	tLogs := make([]*FilteredTransfer, len(transfers))
	for i, trans := range transfers {
		tLogs[i] = ConvertTransfer(trans)
	}
	return tLogs, nil
}

func (t *Transfers) handleFilterTransferLogs(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	transFilter := new(transferdb.TransferFilter)
	if err := json.Unmarshal(res, &transFilter); err != nil {
		return err
	}
	order := req.URL.Query().Get("order")
	if order != string(logdb.DESC) {
		transFilter.Order = transferdb.ASC
	} else {
		transFilter.Order = transferdb.DESC
	}
	tLogs, err := t.filter(transFilter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, tLogs)
}

func (t *Transfers) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleFilterTransferLogs))
}
