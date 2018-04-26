package eventdb_test

import (
	"os"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/eventdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestLogDB(t *testing.T) {
	db, err := eventdb.NewMem()
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
	var events []*eventdb.Event
	for i := 0; i < 100; i++ {
		event := eventdb.NewEvent(header, uint32(i), thor.BytesToBytes32([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), txEvent)
		events = append(events, event)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}

	err = db.Insert(events, nil)
	if err != nil {
		t.Fatal(err)
	}
	limit := 5
	t0 := thor.BytesToBytes32([]byte("topic0"))
	t1 := thor.BytesToBytes32([]byte("topic1"))
	addr := thor.BytesToAddress([]byte("addr"))
	los, err := db.Filter(&eventdb.Filter{
		Range: &eventdb.Range{
			Unit: "Block",
			From: 0,
			To:   10,
		},
		Options: &eventdb.Options{
			Offset: 0,
			Limit:  uint64(limit),
		},
		Order:   "ASC",
		Address: &addr,
		TopicSet: [][5]*thor.Bytes32{{&t0,
			nil,
			nil,
			nil,
			nil},
			{nil,
				&t1,
				nil,
				nil,
				nil}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(los), limit, "limit should be equal")
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

	db, err := eventdb.New(path + "/log.db")
	if err != nil {
		b.Fatal(err)
	}
	l := &tx.Event{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic0")), thor.BytesToBytes32([]byte("topic1"))},
		Data:    []byte("data"),
	}
	var events []*eventdb.Event
	header := new(block.Builder).Build().Header()
	for i := 0; i < 100; i++ {
		event := eventdb.NewEvent(header, uint32(i), thor.BytesToBytes32([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		events = append(events, event)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Insert(events, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
