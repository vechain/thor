package transferdb_test

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
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
	for i := 0; i < 100; i++ {
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
		trans := transferdb.NewTransfer(header, uint32(i), thor.Bytes32{}, from, to, value)
		transfers = append(transfers, trans)
	}
	err = db.Insert(transfers, nil)
	if err != nil {
		t.Fatal(err)
	}
}
