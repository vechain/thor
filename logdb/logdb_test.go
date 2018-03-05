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
	path, err := home()
	if err != nil {
		t.Fatal(err)
	}
	db, err := logdb.OpenDB(path + "/log.db")
	if err != nil {
		t.Fatal(err)
	}
	err = db.ExecInTransaction(logdb.LogSQL)
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
		log := logdb.NewLog(thor.BytesToHash([]byte("blockID")), 1, thor.BytesToHash([]byte("txID")), thor.BytesToAddress([]byte("txOrigin")), l)
		logs = append(logs, log)
	}
	err = db.Insert(logs)
	if err != nil {
		t.Fatal(err)
	}

	t0 := thor.BytesToHash([]byte("topic0"))
	t1 := thor.BytesToHash([]byte("topic1"))
	los, err := db.Filter([]*logdb.FilterOption{{
		FromBlock: 0,
		ToBlock:   1,
		Address:   thor.BytesToAddress([]byte("addr")),
		Topics:    [5]*thor.Hash{&t0, &t1, nil, nil, nil},
	}})
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
