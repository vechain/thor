// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
)

func init() {
	spew.Config.Indent = "    "
	spew.Config.DisableMethods = false
}

// Used for testing
func newEmpty() *Trie {
	db := ethdb.NewMemDatabase()
	trie, _ := New(thor.Bytes32{}, db)
	return trie
}

func TestEmptyTrie(t *testing.T) {
	var trie Trie
	res := trie.Hash()
	exp := emptyRoot
	if res != thor.Bytes32(exp) {
		t.Errorf("expected %x got %x", exp, res)
	}
}

func TestNull(t *testing.T) {
	var trie Trie
	key := make([]byte, 32)
	value := []byte("test")
	trie.Update(key, value)
	if !bytes.Equal(trie.Get(key), value) {
		t.Fatal("wrong value")
	}
}

func TestMissingRoot(t *testing.T) {
	db := ethdb.NewMemDatabase()
	root := thor.Bytes32{1, 2, 3, 4, 5}
	trie, err := New(root, db)
	if trie != nil {
		t.Error("New returned non-nil trie for invalid root")
	}
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("New returned wrong error: %v", err)
	}
}

func TestMissingNode(t *testing.T) {
	db := ethdb.NewMemDatabase()
	trie, _ := New(thor.Bytes32{}, db)
	updateString(trie, "120000", "qwerqwerqwerqwerqwerqwerqwerqwer")
	updateString(trie, "123456", "asdfasdfasdfasdfasdfasdfasdfasdf")
	root, _ := trie.Commit()

	trie, _ = New(root, db)
	_, err := trie.TryGet([]byte("120000"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie, _ = New(root, db)
	_, err = trie.TryGet([]byte("120099"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie, _ = New(root, db)
	_, err = trie.TryGet([]byte("123456"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie, _ = New(root, db)
	err = trie.TryUpdate([]byte("120099"), []byte("zxcvzxcvzxcvzxcvzxcvzxcvzxcvzxcv"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie, _ = New(root, db)
	err = trie.TryDelete([]byte("123456"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	db.Delete(common.FromHex("f4c6f22acf81fd2d993636c74c17d58ad0344b55343f5121bf16fb5f5ec1fc6f"))

	trie, _ = New(root, db)
	_, err = trie.TryGet([]byte("120000"))
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}

	trie, _ = New(root, db)
	_, err = trie.TryGet([]byte("120099"))
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}

	trie, _ = New(root, db)
	_, err = trie.TryGet([]byte("123456"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie, _ = New(root, db)
	err = trie.TryUpdate([]byte("120099"), []byte("zxcv"))
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}

	trie, _ = New(root, db)
	err = trie.TryDelete([]byte("123456"))
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}
}

func TestInsert(t *testing.T) {
	trie := newEmpty()

	updateString(trie, "doe", "reindeer")
	updateString(trie, "dog", "puppy")
	updateString(trie, "dogglesworth", "cat")

	exp, _ := thor.ParseBytes32("6ca394ff9b13d6690a51dea30b1b5c43108e52944d30b9095227c49bae03ff8b")
	root := trie.Hash()
	if root != exp {
		t.Errorf("exp %v got %v", exp, root)
	}

	trie = newEmpty()
	updateString(trie, "A", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	exp, _ = thor.ParseBytes32("e9d7f23f40cd82fe35f5a7a6778c3503f775f3623ba7a71fb335f0eee29dac8a")
	root, err := trie.Commit()
	if err != nil {
		t.Fatalf("commit error: %v", err)
	}
	if root != exp {
		t.Errorf("exp %v got %v", exp, root)
	}
}

func TestGet(t *testing.T) {
	trie := newEmpty()
	updateString(trie, "doe", "reindeer")
	updateString(trie, "dog", "puppy")
	updateString(trie, "dogglesworth", "cat")

	for i := 0; i < 2; i++ {
		res := getString(trie, "dog")
		if !bytes.Equal(res, []byte("puppy")) {
			t.Errorf("expected puppy got %x", res)
		}

		unknown := getString(trie, "unknown")
		if unknown != nil {
			t.Errorf("expected nil got %x", unknown)
		}

		if i == 1 {
			return
		}
		trie.Commit()
	}
}

func TestDelete(t *testing.T) {
	trie := newEmpty()
	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"ether", ""},
		{"dog", "puppy"},
		{"shaman", ""},
	}
	for _, val := range vals {
		if val.v != "" {
			updateString(trie, val.k, val.v)
		} else {
			deleteString(trie, val.k)
		}
	}

	hash := trie.Hash()
	exp, _ := thor.ParseBytes32("79a9b42da0e261b9f3ca9e78560ac8d486bcce2da8a5ddb2df8721d4c0dc2d0a")
	if hash != exp {
		t.Errorf("expected %v got %v", exp, hash)
	}
}

func TestEmptyValues(t *testing.T) {
	trie := newEmpty()

	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"ether", ""},
		{"dog", "puppy"},
		{"shaman", ""},
	}
	for _, val := range vals {
		updateString(trie, val.k, val.v)
	}

	hash := trie.Hash()
	exp, _ := thor.ParseBytes32("79a9b42da0e261b9f3ca9e78560ac8d486bcce2da8a5ddb2df8721d4c0dc2d0a")
	if hash != exp {
		t.Errorf("expected %v got %v", exp, hash)
	}
}

func TestReplication(t *testing.T) {
	trie := newEmpty()
	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"dog", "puppy"},
		{"somethingveryoddindeedthis is", "myothernodedata"},
	}
	for _, val := range vals {
		updateString(trie, val.k, val.v)
	}
	exp, err := trie.Commit()
	if err != nil {
		t.Fatalf("commit error: %v", err)
	}

	// create a new trie on top of the database and check that lookups work.
	trie2, err := New(exp, trie.db)
	if err != nil {
		t.Fatalf("can't recreate trie at %x: %v", exp, err)
	}
	for _, kv := range vals {
		if string(getString(trie2, kv.k)) != kv.v {
			t.Errorf("trie2 doesn't have %q => %q", kv.k, kv.v)
		}
	}
	hash, err := trie2.Commit()
	if err != nil {
		t.Fatalf("commit error: %v", err)
	}
	if hash != exp {
		t.Errorf("root failure. expected %x got %x", exp, hash)
	}

	// perform some insertions on the new trie.
	vals2 := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		// {"shaman", "horse"},
		// {"doge", "coin"},
		// {"ether", ""},
		// {"dog", "puppy"},
		// {"somethingveryoddindeedthis is", "myothernodedata"},
		// {"shaman", ""},
	}
	for _, val := range vals2 {
		updateString(trie2, val.k, val.v)
	}
	if hash := trie2.Hash(); hash != exp {
		t.Errorf("root failure. expected %x got %x", exp, hash)
	}
}

func TestLargeValue(t *testing.T) {
	trie := newEmpty()
	trie.Update([]byte("key1"), []byte{99, 99, 99, 99})
	trie.Update([]byte("key2"), bytes.Repeat([]byte{1}, 32))
	trie.Hash()
}

type countingDB struct {
	Database
	gets map[string]int
}

func (db *countingDB) Get(key []byte) ([]byte, error) {
	db.gets[string(key)]++
	return db.Database.Get(key)
}

// TestCacheUnload checks that decoded nodes are unloaded after a
// certain number of commit operations.
// func TestCacheUnload(t *testing.T) {
// 	// Create test trie with two branches.
// 	trie := newEmpty()
// 	key1 := "---------------------------------"
// 	key2 := "---some other branch"
// 	updateString(trie, key1, "this is the branch of key1.")
// 	updateString(trie, key2, "this is the branch of key2.")
// 	root, _ := trie.Commit()

// 	// Commit the trie repeatedly and access key1.
// 	// The branch containing it is loaded from DB exactly two times:
// 	// in the 0th and 6th iteration.
// 	db := &countingDB{Database: trie.db, gets: make(map[string]int)}
// 	trie, _ = New(root, db)
// 	trie.SetCacheLimit(5)
// 	for i := 0; i < 12; i++ {
// 		getString(trie, key1)
// 		trie.Commit()
// 	}

// 	// Check that it got loaded two times.
// 	for dbkey, count := range db.gets {
// 		if count != 2 {
// 			t.Errorf("db key %x loaded %d times, want %d times", []byte(dbkey), count, 2)
// 		}
// 	}
// }

// randTest performs random trie operations.
// Instances of this test are created by Generate.
type randTest []randTestStep

type randTestStep struct {
	op    int
	key   []byte // for opUpdate, opDelete, opGet
	value []byte // for opUpdate
	err   error  // for debugging
}

const (
	opUpdate = iota
	opDelete
	opGet
	opCommit
	opHash
	opReset
	opItercheckhash
	opCheckCacheInvariant
	opMax // boundary value, not an actual op
)

func (randTest) Generate(r *rand.Rand, size int) reflect.Value {
	var allKeys [][]byte
	genKey := func() []byte {
		if len(allKeys) < 2 || r.Intn(100) < 10 {
			// new key
			key := make([]byte, r.Intn(50))
			r.Read(key)
			allKeys = append(allKeys, key)
			return key
		}
		// use existing key
		return allKeys[r.Intn(len(allKeys))]
	}

	var steps randTest
	for i := 0; i < size; i++ {
		step := randTestStep{op: r.Intn(opMax)}
		switch step.op {
		case opUpdate:
			step.key = genKey()
			step.value = make([]byte, 8)
			binary.BigEndian.PutUint64(step.value, uint64(i))
		case opGet, opDelete:
			step.key = genKey()
		}
		steps = append(steps, step)
	}
	return reflect.ValueOf(steps)
}

func runRandTest(rt randTest) bool {
	db := ethdb.NewMemDatabase()
	tr, _ := New(thor.Bytes32{}, db)
	values := make(map[string]string) // tracks content of the trie

	for i, step := range rt {
		switch step.op {
		case opUpdate:
			tr.Update(step.key, step.value)
			values[string(step.key)] = string(step.value)
		case opDelete:
			tr.Delete(step.key)
			delete(values, string(step.key))
		case opGet:
			v := tr.Get(step.key)
			want := values[string(step.key)]
			if string(v) != want {
				rt[i].err = fmt.Errorf("mismatch for key 0x%x, got 0x%x want 0x%x", step.key, v, want)
			}
		case opCommit:
			_, rt[i].err = tr.Commit()
		case opHash:
			tr.Hash()
		case opReset:
			hash, err := tr.Commit()
			if err != nil {
				rt[i].err = err
				return false
			}
			newtr, err := New(hash, db)
			if err != nil {
				rt[i].err = err
				return false
			}
			tr = newtr
		case opItercheckhash:
			checktr, _ := New(thor.Bytes32{}, nil)
			it := NewIterator(tr.NodeIterator(nil))
			for it.Next() {
				checktr.Update(it.Key, it.Value)
			}
			if tr.Hash() != checktr.Hash() {
				rt[i].err = fmt.Errorf("hash mismatch in opItercheckhash")
			}
		case opCheckCacheInvariant:
			// rt[i].err = checkCacheInvariant(tr.root, nil, tr.cachegen, false, 0)
		}
		// Abort the test on error.
		if rt[i].err != nil {
			return false
		}
	}
	return true
}

// func checkCacheInvariant(n, parent node, parentCachegen uint16, parentDirty bool, depth int) error {
// 	var children []node
// 	var flag nodeFlag
// 	switch n := n.(type) {
// 	case *shortNode:
// 		flag = n.flags
// 		children = []node{n.Val}
// 	case *fullNode:
// 		flag = n.flags
// 		children = n.Children[:]
// 	default:
// 		return nil
// 	}

// 	errorf := func(format string, args ...interface{}) error {
// 		msg := fmt.Sprintf(format, args...)
// 		msg += fmt.Sprintf("\nat depth %d node %s", depth, spew.Sdump(n))
// 		msg += fmt.Sprintf("parent: %s", spew.Sdump(parent))
// 		return errors.New(msg)
// 	}
// 	if flag.gen > parentCachegen {
// 		return errorf("cache invariant violation: %d > %d\n", flag.gen, parentCachegen)
// 	}
// 	if depth > 0 && !parentDirty && flag.dirty {
// 		return errorf("cache invariant violation: %d > %d\n", flag.gen, parentCachegen)
// 	}
// 	for _, child := range children {
// 		if err := checkCacheInvariant(child, n, flag.gen, flag.dirty, depth+1); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

func TestRandom(t *testing.T) {
	if err := quick.Check(runRandTest, nil); err != nil {
		if cerr, ok := err.(*quick.CheckError); ok {
			t.Fatalf("random test iteration %d failed: %s", cerr.Count, spew.Sdump(cerr.In))
		}
		t.Fatal(err)
	}
}

func BenchmarkGet(b *testing.B)      { benchGet(b, false) }
func BenchmarkGetDB(b *testing.B)    { benchGet(b, true) }
func BenchmarkUpdateBE(b *testing.B) { benchUpdate(b, binary.BigEndian) }
func BenchmarkUpdateLE(b *testing.B) { benchUpdate(b, binary.LittleEndian) }

const benchElemCount = 20000

func benchGet(b *testing.B, commit bool) {
	trie := new(Trie)
	if commit {
		_, tmpdb := tempDB()
		trie, _ = New(thor.Bytes32{}, tmpdb)
	}
	k := make([]byte, 32)
	for i := 0; i < benchElemCount; i++ {
		binary.LittleEndian.PutUint64(k, uint64(i))
		trie.Update(k, k)
	}
	binary.LittleEndian.PutUint64(k, benchElemCount/2)
	if commit {
		trie.Commit()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Get(k)
	}
	b.StopTimer()

	if commit {
		ldb := trie.db.(*ethdb.LDBDatabase)
		ldb.Close()
		os.RemoveAll(ldb.Path())
	}
}

func benchUpdate(b *testing.B, e binary.ByteOrder) *Trie {
	trie := newEmpty()
	k := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		e.PutUint64(k, uint64(i))
		trie.Update(k, k)
	}
	return trie
}

// Benchmarks the trie hashing. Since the trie caches the result of any operation,
// we cannot use b.N as the number of hashing rouns, since all rounds apart from
// the first one will be NOOP. As such, we'll use b.N as the number of account to
// insert into the trie before measuring the hashing.
func BenchmarkHash(b *testing.B) {
	// Make the random benchmark deterministic
	random := rand.New(rand.NewSource(0))

	// Create a realistic account trie to hash
	addresses := make([][20]byte, b.N)
	for i := 0; i < len(addresses); i++ {
		for j := 0; j < len(addresses[i]); j++ {
			addresses[i][j] = byte(random.Intn(256))
		}
	}
	accounts := make([][]byte, len(addresses))
	for i := 0; i < len(accounts); i++ {
		var (
			nonce   = uint64(random.Int63())
			balance = new(big.Int).Rand(random, new(big.Int).Exp(common.Big2, common.Big256, nil))
			root    = emptyRoot
			code    = crypto.Keccak256(nil)
		)
		accounts[i], _ = rlp.EncodeToBytes([]interface{}{nonce, balance, root, code})
	}
	// Insert the accounts into the trie and hash it
	trie := newEmpty()
	for i := 0; i < len(addresses); i++ {
		trie.Update(thor.Blake2b(addresses[i][:]).Bytes(), accounts[i])
	}
	b.ResetTimer()
	b.ReportAllocs()
	trie.Hash()
}

func tempDB() (string, Database) {
	dir, err := ioutil.TempDir("", "trie-bench")
	if err != nil {
		panic(fmt.Sprintf("can't create temporary directory: %v", err))
	}
	db, err := ethdb.NewLDBDatabase(dir, 256, 0)
	if err != nil {
		panic(fmt.Sprintf("can't create temporary database: %v", err))
	}
	return dir, db
}

func getString(trie *Trie, k string) []byte {
	return trie.Get([]byte(k))
}

func updateString(trie *Trie, k, v string) {
	trie.Update([]byte(k), []byte(v))
}

func deleteString(trie *Trie, k string) {
	trie.Delete([]byte(k))
}

func TestExtended(t *testing.T) {
	db := ethdb.NewMemDatabase()
	tr := NewExtended(thor.Bytes32{}, 0, db, false)

	vals1 := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"dog", "puppy"},
		{"somethingveryoddindeedthis is", "myothernodedata"},
	}

	vals2 := []struct{ k, v string }{
		{"do1", "verb1"},
		{"ether1", "wookiedoo1"},
		{"horse1", "stallion1"},
		{"shaman1", "horse1"},
		{"doge1", "coin1"},
		{"dog1", "puppy1"},
		{"somethingveryoddindeedthis is1", "myothernodedata1"},
		{"foo", "verb2"},
		{"bar", "wookiedoo2"},
		{"baz", "stallion2"},
		{"hello", "horse2"},
		{"world", "coin2"},
		{"ethereum", "puppy2"},
		{"is good", "myothernodedata2"},
	}

	for _, v := range vals1 {
		tr.Update([]byte(v.k), []byte(v.v), thor.Blake2b([]byte(v.v)).Bytes())
	}

	root1, err := tr.Commit(1)
	if err != nil {
		t.Errorf("commit failed %v", err)
	}

	for _, v := range vals2 {
		tr.Update([]byte(v.k), []byte(v.v), thor.Blake2b([]byte(v.v)).Bytes())
	}
	root2, err := tr.Commit(2)
	if err != nil {
		t.Errorf("commit failed %v", err)
	}

	tr1 := NewExtended(root1, 1, db, false)
	for _, v := range vals1 {
		val, meta, _ := tr1.Get([]byte(v.k))
		if string(val) != v.v {
			t.Errorf("incorrect value for key '%v'", v.k)
		}
		if string(meta) != string(thor.Blake2b(val).Bytes()) {
			t.Errorf("incorrect value meta for key '%v'", v.k)
		}
	}

	tr2 := NewExtended(root2, 2, db, false)
	for _, v := range append(vals1, vals2...) {
		val, meta, _ := tr2.Get([]byte(v.k))
		if string(val) != v.v {
			t.Errorf("incorrect value for key '%v'", v.k)
		}
		if string(meta) != string(thor.Blake2b(val).Bytes()) {
			t.Errorf("incorrect value meta for key '%v'", v.k)
		}
	}
}

type kedb struct {
	*ethdb.MemDatabase
}

func (db *kedb) Encode(hash []byte, seq uint64, path []byte) []byte {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], seq)
	return append(k[:], path...)
}

func TestNonCryptoExtended(t *testing.T) {
	db := &kedb{ethdb.NewMemDatabase()}

	tr := NewExtended(thor.Bytes32{}, 0, db, true)
	var root thor.Bytes32
	n := uint32(100)
	for i := uint32(0); i < n; i++ {
		var k [4]byte
		binary.BigEndian.PutUint32(k[:], i)
		tr.Update(k[:], thor.Blake2b(k[:]).Bytes(), nil)
		root, _ = tr.Commit(uint64(i))
	}

	tr = NewExtended(root, uint64(n-1), db, true)
	for i := uint32(0); i < n; i++ {
		var k [4]byte
		binary.BigEndian.PutUint32(k[:], i)
		val, _, err := tr.Get(k[:])
		assert.Nil(t, err)
		assert.Equal(t, thor.Blake2b(k[:]).Bytes(), val)
	}
}

func TestExtendedCached(t *testing.T) {
	db := ethdb.NewMemDatabase()
	tr := NewExtended(thor.Bytes32{}, 0, db, false)

	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"dog", "puppy"},
	}
	for _, val := range vals {
		tr.Update([]byte(val.k), []byte(val.v), nil)
	}

	tr = NewExtendedCached(tr.RootNode(), db, false)

	for _, val := range vals {
		v, _, _ := tr.Get([]byte(val.k))
		if val.v != string(v) {
			t.Errorf("incorrect value for key '%v'", val.k)
		}
	}
}
