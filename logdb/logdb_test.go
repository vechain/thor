package logdb_test

import (
	"fmt"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"os"
	"os/user"
	"testing"
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

	l := &tx.Log{
		Address: thor.BytesToAddress([]byte("addr")),
		Topics:  []thor.Hash{thor.BytesToHash([]byte("topic0")), thor.BytesToHash([]byte("topic1"))},
		Data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 97, 48},
	}

	var logs []*logdb.Log
	for i := 0; i < 100; i++ {
		log := logdb.NewLog(thor.BytesToHash([]byte("blockID")), 1, uint32(i), thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		logs = append(logs, log)
	}
	err = db.Insert(logs)
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
	for i := 0; i < 100; i++ {
		log := logdb.NewLog(thor.BytesToHash([]byte("blockID")), 1, uint32(i), thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		logs = append(logs, log)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Insert(logs)
		if err != nil {
			b.Fatal(err)
		}
	}
}
