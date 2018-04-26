package events

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/eventdb"
	"github.com/vechain/thor/thor"
)

type Events struct {
	db *eventdb.EventDB
}

func New(db *eventdb.EventDB) *Events {
	return &Events{
		db,
	}
}

//Filter query events with option
func (e *Events) filter(filter *Filter) ([]*FilteredEvent, error) {
	f := convertFilter(filter)
	events, err := e.db.Filter(f)
	if err != nil {
		return nil, err
	}
	fes := make([]*FilteredEvent, len(events))
	for i, e := range events {
		fes[i] = convertEvent(e)
	}
	return fes, nil
}

func (e *Events) handleFilter(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	var filter Filter
	if err := json.Unmarshal(res, &filter); err != nil {
		return err
	}
	query := req.URL.Query()
	if query.Get("address") != "" {
		addr, err := thor.ParseAddress(query.Get("address"))
		if err != nil {
			return utils.BadRequest(err, "address")
		}
		filter.Address = &addr
	}
	order := query.Get("order")
	if order != string(eventdb.DESC) {
		filter.Order = eventdb.ASC
	} else {
		filter.Order = eventdb.DESC
	}
	fes, err := e.filter(&filter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, fes)
}

func (e *Events) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(e.handleFilter))
}
