package transferdb_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/tx"
)

func TestTransferDB(t *testing.T) {
	db, err := transferdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	from := thor.BytesToAddress([]byte("from"))
	to := thor.BytesToAddress([]byte("to"))
	value := big.NewInt(10)
	header := new(block.Builder).Build().Header()
	var transfers []*transferdb.Transfer
	count := 100
	for i := 0; i < count; i++ {
		transLog := &tx.Transfer{
			Sender:    from,
			Recipient: to,
			Amount:    value,
		}
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
		trans := transferdb.NewTransfer(header, uint32(i), thor.Bytes32{}, from, transLog)
		transfers = append(transfers, trans)
	}
	err = db.Insert(transfers, nil)
	if err != nil {
		t.Fatal(err)
	}

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
			Limit:  uint64(count),
		},
		Order: transferdb.DESC,
	}
	ts, err := db.Filter(tf)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(ts), count, "transfers searched")
}
