package transfers_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/transfers"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/tx"
)

func TestTransfers(t *testing.T) {
	ts := initLogServer(t)
	defer ts.Close()
	getTransfers(t, ts)
}

func getTransfers(t *testing.T, ts *httptest.Server) {
	limit := 5
	from := thor.BytesToAddress([]byte("from"))
	to := thor.BytesToAddress([]byte("to"))
	tf := &transferdb.TransferFilter{
		AddressSets: []*transferdb.AddressSet{
			&transferdb.AddressSet{
				From: &from,
				To:   &to,
			},
		},
		Range: &transferdb.Range{
			Unit: transferdb.Block,
			From: 0,
			To:   1000,
		},
		Options: &transferdb.Options{
			Offset: 0,
			Limit:  uint64(limit),
		},
		Order: transferdb.DESC,
	}
	f, err := json.Marshal(tf)
	if err != nil {
		t.Fatal(err)
	}
	res := httpPost(t, ts.URL+"/transfers", f)
	var tLogs []*transfers.FilteredTransfer
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, limit, len(tLogs), "should be `limit` transfers")
}

func initLogServer(t *testing.T) *httptest.Server {
	db, err := transferdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}

	from := thor.BytesToAddress([]byte("from"))
	to := thor.BytesToAddress([]byte("to"))
	value := big.NewInt(10)
	header := new(block.Builder).Build().Header()
	var trs []*transferdb.Transfer
	count := 100
	for i := 0; i < count; i++ {
		transLog := &tx.Transfer{
			Sender:    from,
			Recipient: to,
			Amount:    value,
		}
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
		trans := transferdb.NewTransfer(header, uint32(i), thor.Bytes32{}, from, transLog)
		trs = append(trs, trans)
	}
	err = db.Insert(trs, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	transfers.New(db).Mount(router, "/transfers")
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
