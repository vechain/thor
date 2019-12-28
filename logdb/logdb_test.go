// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb_test

import (
	"context"
	"crypto/rand"
	"math/big"
	"os"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	logdb "github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func newTx() *tx.Transaction {
	tx := new(tx.Builder).Build()
	var sig [65]byte
	rand.Read(sig[:])
	return tx.WithSignature(sig[:])
}

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

	b := new(block.Builder).Transaction(newTx()).Build()

	for i := 0; i < 100; i++ {
		if err := db.Log(func(w *logdb.Writer) error {
			return w.Write(b, tx.Receipts{
				{Outputs: []*tx.Output{{
					Events:    tx.Events{txEvent},
					Transfers: nil,
				}}},
			})
		}); err != nil {
			t.Fatal(err)
		}

		b = new(block.Builder).ParentID(b.Header().ID()).Transaction(newTx()).Build()
	}

	limit := 5
	t0 := thor.BytesToBytes32([]byte("topic0"))
	t1 := thor.BytesToBytes32([]byte("topic1"))
	addr := thor.BytesToAddress([]byte("addr"))
	es, err := db.FilterEvents(context.Background(), &logdb.EventFilter{
		Range: &logdb.Range{
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
	b := new(block.Builder).Transaction(newTx()).Build()
	count := 100
	for i := 0; i < count; i++ {
		transLog := &tx.Transfer{
			Sender:    from,
			Recipient: to,
			Amount:    value,
		}
		if err := db.Log(func(w *logdb.Writer) error {
			return w.Write(b, tx.Receipts{
				{Outputs: []*tx.Output{{
					Transfers: tx.Transfers{transLog},
				}}},
			})
		}); err != nil {
			t.Fatal(err)
		}
		b = new(block.Builder).ParentID(b.Header().ID()).Transaction(newTx()).Build()
	}

	tf := &logdb.TransferFilter{
		CriteriaSet: []*logdb.TransferCriteria{
			&logdb.TransferCriteria{
				Sender:    &from,
				Recipient: &to,
			},
		},
		Range: &logdb.Range{
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
	assert.Equal(t, count, len(ts), "transfers searched")
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
