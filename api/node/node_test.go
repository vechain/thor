package node_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/node"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/txpool"
)

func TestNode(t *testing.T) {
	ts := initCommServer(t)
	res := httpGet(t, ts.URL+"/node/network/peers")
	var peersStats map[string]string
	if err := json.Unmarshal(res, &peersStats); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 0, len(peersStats), "count should be zero")
}

func initCommServer(t *testing.T) *httptest.Server {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	gene, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := gene.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	comm := comm.New(chain, txpool.New(chain, stateC))
	router := mux.NewRouter()
	node.New(comm).Mount(router, "/node")
	ts := httptest.NewServer(router)
	return ts
}

func httpGet(t *testing.T, url string) []byte {
	res, err := http.Get(url)
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
