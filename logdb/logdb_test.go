package logdb_test

import (
	"fmt"
	"os"
	"os/user"
	"testing"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestLogDB(t *testing.T) {
	// path, err := home()
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// db, err := logdb.New(path + "/log.db")
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	l := &tx.Log{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Hash{thor.BytesToHash([]byte("topic0")), thor.BytesToHash([]byte("topic1"))},
		Data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 97, 48},
	}

	header := new(block.Builder).Build().Header()
	var logs []*logdb.Log
	for i := 0; i < 100; i++ {
		log := logdb.NewLog(header, uint32(i), thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		logs = append(logs, log)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}
	err = db.Insert(logs...)
	if err != nil {
		t.Fatal(err)
	}

	t0 := thor.BytesToHash([]byte("topic0"))
	t1 := thor.BytesToHash([]byte("topic1"))
	addr := thor.BytesToAddress([]byte("addr"))
	los, err := db.Filter(&logdb.FilterOption{
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
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(los)
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
	l := &tx.Log{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Hash{thor.BytesToHash([]byte("topic0")), thor.BytesToHash([]byte("topic1"))},
		Data:    []byte("data"),
	}
	var logs []*logdb.Log
	header := new(block.Builder).Build().Header()
	for i := 0; i < 100; i++ {
		log := logdb.NewLog(header, uint32(i), thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		logs = append(logs, log)
		header = new(block.Builder).ParentID(header.ID()).Build().Header()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Insert(logs...)
		if err != nil {
			b.Fatal(err)
		}
	}
}
