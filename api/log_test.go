package api_test

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"net/http/httptest"
	"os"
	"os/user"
	"testing"
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
	var logs []*types.Log
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

	var logs []*logdb.Log
	for i := 0; i < 2; i++ {
		log := logdb.NewLog(thor.BytesToHash([]byte("blockID")), 1, uint32(i), thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		logs = append(logs, log)
	}
	err = db.Insert(logs)
	if err != nil {
		t.Fatal(err)
	}
	li := api.NewLogInterface(db)
	router := mux.NewRouter()
	api.NewLogHTTPRouter(router, li)
	ts := httptest.NewServer(router)
	return ts
}
func home() (string, error) {
	// try to get HOME env
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	//
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	if user.HomeDir != "" {
		return user.HomeDir, nil
	}

	return os.Getwd()
}
