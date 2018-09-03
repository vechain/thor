package events

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/logdb"
)

type Events2 struct {
	db *logdb.LogDB
}

func New2(db *logdb.LogDB) *Events2 {
	return &Events2{
		db,
	}
}

//Filter query events with option
func (e *Events2) filter(ctx context.Context, filter *logdb.EventFilter) ([]*FilteredEvent, error) {
	events, err := e.db.FilterEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	fes := make([]*FilteredEvent, len(events))
	for i, e := range events {
		fes[i] = convertEvent(e)
	}
	return fes, nil
}

func (e *Events2) handleFilter(w http.ResponseWriter, req *http.Request) error {
	var filter *logdb.EventFilter
	if err := utils.ParseJSON(req.Body, &filter); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	fes, err := e.filter(req.Context(), filter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, fes)
}

func (e *Events2) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(e.handleFilter))
}
