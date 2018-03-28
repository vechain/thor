package logs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/logs"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestLog(t *testing.T) {
	ts := initLogServer(t)
	t0 := thor.BytesToHash([]byte("topic0"))
	t1 := thor.BytesToHash([]byte("topic1"))
	addr := thor.BytesToAddress([]byte("addr"))
	op := &logdb.FilterOption{
		FromBlock: 0,
		ToBlock:   1,
		Address:   &addr,
		TopicSet: [][5]*thor.Hash{{&t0,
			nil,
			nil,
			nil,
			nil},
			{nil,
				&t1,
				nil,
				nil,
				nil}},
	}
	ops, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	r, err := httpPost(ts, ts.URL+"/logs", ops)
	if err != nil {
		t.Fatal(err)
	}
	var logs []*logs.Log
	if err := json.Unmarshal(r, &logs); err != nil {
		t.Fatal(err)
	}
	fmt.Println(logs)
}

func initLogServer(t *testing.T) *httptest.Server {
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	l := &tx.Log{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Hash{thor.BytesToHash([]byte("topic0")), thor.BytesToHash([]byte("topic1"))},
		Data:    []byte("data"),
	}

	header := new(block.Builder).Build().Header()
	var lgs []*logdb.Log
	for i := 0; i < 100; i++ {
		log := logdb.NewLog(header, uint32(i), thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		lgs = append(lgs, log)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}
	err = db.Insert(lgs...)
	if err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	logs.New(db).Mount(router, "/logs")
	ts := httptest.NewServer(router)
	return ts
}

func httpPost(ts *httptest.Server, url string, data []byte) ([]byte, error) {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	return r, nil
}
