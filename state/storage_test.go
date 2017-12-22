package state_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestStorage(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	storage := state.NewStorage(db)
	keystr := "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b422"
	valuestr := "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b423"
	root, _ := cry.ParseHash(emptyRootHash)
	key, _ := cry.ParseHash(keystr)
	value, _ := cry.ParseHash(valuestr)
	storage.Update(*root, *key, *value)
	v, _ := storage.Get(*root, *key)
	assert.Equal(t, *value, v, "storage value should be euqal")
}
