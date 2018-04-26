package events_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/events"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/eventdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var contractAddr = thor.BytesToAddress([]byte("contract"))

func TestEvents(t *testing.T) {
	ts := initEventServer(t)
	defer ts.Close()
	getEvents(t, ts)
}

func getEvents(t *testing.T, ts *httptest.Server) {
	t0 := thor.BytesToBytes32([]byte("topic0"))
	t1 := thor.BytesToBytes32([]byte("topic1"))
	limit := 5
	filter := &events.Filter{
		Range: &eventdb.Range{
			Unit: "",
			From: 0,
			To:   10,
		},
		Options: &eventdb.Options{
			Offset: 0,
			Limit:  uint64(limit),
		},
		Order:   "",
		Address: &contractAddr,
		TopicSets: []*events.TopicSet{
			&events.TopicSet{
				Topic0: &t0,
			},
			&events.TopicSet{
				Topic1: &t1,
			},
		},
	}
	f, err := json.Marshal(&filter)
	if err != nil {
		t.Fatal(err)
	}
	res := httpPost(t, ts.URL+"/events?address="+contractAddr.String(), f)
	var logs []*events.FilteredEvent
	if err := json.Unmarshal(res, &logs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, limit, len(logs), "should be `limit` logs")
}

func initEventServer(t *testing.T) *httptest.Server {
	db, err := eventdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	txEv := &tx.Event{
		Address: contractAddr,
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic0")), thor.BytesToBytes32([]byte("topic1"))},
		Data:    []byte("data"),
	}

	header := new(block.Builder).Build().Header()
	var evs []*eventdb.Event
	for i := 0; i < 100; i++ {
		ev := eventdb.NewEvent(header, uint32(i), thor.BytesToBytes32([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), txEv)
		evs = append(evs, ev)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}
	err = db.Insert(evs, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	events.New(db).Mount(router, "/events")
	ts := httptest.NewServer(router)
	return ts
}

func httpPost(t *testing.T, url string, data []byte) []byte {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
