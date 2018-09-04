// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb_test

import (
	"context"
	"math/big"
	"os"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestEvents(t *testing.T) {
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	txEvent := &tx.Event{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic0")), thor.BytesToBytes32([]byte("topic1"))},
		Data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 97, 48},
	}

	header := new(block.Builder).Build().Header()

	for i := 0; i < 100; i++ {
		if err := db.Prepare(header).ForTransaction(thor.BytesToBytes32([]byte("txID")), thor.BytesToAddress([]byte("txOrigin"))).
			Insert(tx.Events{txEvent}, nil).Commit(); err != nil {
			t.Fatal(err)
		}

		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}

	limit := 5
	t0 := thor.BytesToBytes32([]byte("topic0"))
	t1 := thor.BytesToBytes32([]byte("topic1"))
	addr := thor.BytesToAddress([]byte("addr"))
	es, err := db.FilterEvents(context.Background(), &logdb.EventFilter{
		Range: &logdb.Range{
			Unit: logdb.Block,
			From: 0,
			To:   10,
		},
		Options: &logdb.Options{
			Offset: 0,
			Limit:  uint64(limit),
		},
		Order: logdb.DESC,
		CriteriaSet: []*logdb.EventCriteria{
			&logdb.EventCriteria{
				Address: &addr,
				Topics: [5]*thor.Bytes32{nil,
					nil,
					nil,
					nil,
					nil},
			},
			&logdb.EventCriteria{
				Address: &addr,
				Topics: [5]*thor.Bytes32{&t0,
					&t1,
					nil,
					nil,
					nil},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(es), limit, "limit should be equal")
}

func TestTransfers(t *testing.T) {
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	from := thor.BytesToAddress([]byte("from"))
	to := thor.BytesToAddress([]byte("to"))
	value := big.NewInt(10)
	header := new(block.Builder).Build().Header()
	count := 100
	for i := 0; i < count; i++ {
		transLog := &tx.Transfer{
			Sender:    from,
			Recipient: to,
			Amount:    value,
		}
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
		if err := db.Prepare(header).ForTransaction(thor.Bytes32{}, from).Insert(nil, tx.Transfers{transLog}).
			Commit(); err != nil {
			t.Fatal(err)
		}

	}

	tf := &logdb.TransferFilter{
		CriteriaSet: []*logdb.TransferCriteria{
			&logdb.TransferCriteria{
				TxOrigin:  &from,
				Recipient: &to,
			},
		},
		Range: &logdb.Range{
			Unit: logdb.Block,
			From: 0,
			To:   1000,
		},
		Options: &logdb.Options{
			Offset: 0,
			Limit:  uint64(count),
		},
		Order: logdb.DESC,
	}
	ts, err := db.FilterTransfers(context.Background(), tf)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(ts), count, "transfers searched")
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

func BenchmarkLog(b *testing.B) {
	path, err := home()
	if err != nil {
		b.Fatal(err)
	}

	db, err := logdb.New(path + "/log.db")
	if err != nil {
		b.Fatal(err)
	}
	l := &tx.Event{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic0")), thor.BytesToBytes32([]byte("topic1"))},
		Data:    []byte("data"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		header := new(block.Builder).Build().Header()
		batch := db.Prepare(header)
		txBatch := batch.ForTransaction(thor.BytesToBytes32([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")))
		for j := 0; j < 100; j++ {
			txBatch.Insert(tx.Events{l}, nil)
			header = new(block.Builder).ParentID(header.ID()).Build().Header()
		}

		if err := batch.Commit(); err != nil {
			b.Fatal(err)
		}
	}
}
