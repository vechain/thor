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
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var contractAddr = thor.BytesToAddress([]byte("contract"))

func TestEvents(t *testing.T) {
	ts := initLogServer(t)
	defer ts.Close()
	getLogs(t, ts)
}

func getLogs(t *testing.T, ts *httptest.Server) {
	t0 := thor.BytesToBytes32([]byte("topic0"))
	t1 := thor.BytesToBytes32([]byte("topic1"))
	limit := 5
	logFilter := &events.LogFilter{
		Range: &logdb.Range{
			Unit: "",
			From: 0,
			To:   10,
		},
		Options: &logdb.Options{
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
	f, err := json.Marshal(logFilter)
	if err != nil {
		t.Fatal(err)
	}
	res := httpPost(t, ts.URL+"/logs?address="+contractAddr.String(), f)
	var logs []*events.FilteredLog
	if err := json.Unmarshal(res, &logs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, limit, len(logs), "should be `limit` logs")
}

func initLogServer(t *testing.T) *httptest.Server {
	logDB, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	l := &tx.Log{
		Address: contractAddr,
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic0")), thor.BytesToBytes32([]byte("topic1"))},
		Data:    []byte("data"),
	}

	header := new(block.Builder).Build().Header()
	var lgs []*logdb.Log
	for i := 0; i < 100; i++ {
		log := logdb.NewLog(header, uint32(i), thor.BytesToBytes32([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		lgs = append(lgs, log)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}
	err = logDB.Insert(lgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	events.New(logDB).Mount(router, "/logs")
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
