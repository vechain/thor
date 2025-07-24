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
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

func init() {
	spew.Config.Indent = "    "
	spew.Config.DisableMethods = false
}

func makeKey(path []byte, ver Version) []byte {
	key := binary.AppendUvarint([]byte(nil), uint64(ver.Major))
	key = binary.AppendUvarint(key, uint64(ver.Minor))
	return append(key, path...)
}

type memdb struct {
	db *ethdb.MemDatabase
}

func (m *memdb) Get(path []byte, ver Version) ([]byte, error) {
	return m.db.Get(makeKey(path, ver))
}

func (m *memdb) Put(path []byte, ver Version, value []byte) error {
	return m.db.Put(makeKey(path, ver), value)
}

func newMemDatabase() *memdb {
	return &memdb{ethdb.NewMemDatabase()}
}

func TestEmptyTrie(t *testing.T) {
	var trie Trie
	res := trie.Hash()

	if res != emptyRoot {
		t.Errorf("expected %x got %x", emptyRoot, res)
	}
}

func TestNull(t *testing.T) {
	var trie Trie
	key := make([]byte, 32)
	value := []byte("test")
	trie.Update(key, value, nil)
	gotVal, _, _ := trie.Get(key)
	if !bytes.Equal(gotVal, value) {
		t.Fatal("wrong value")
	}
}

func TestMissingRoot(t *testing.T) {
	db := newMemDatabase()
	hash := thor.Bytes32{1, 2, 3, 4, 5}
	trie := New(Root{Hash: hash}, db)

	// will resolve node
	err := trie.Commit(db, Version{}, false)
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("New returned wrong error: %v", err)
	}
}

func TestMissingNode(t *testing.T) {
	db := newMemDatabase()

	root := Root{}
	trie := New(root, db)
	updateString(trie, "120000", "qwerqwerqwerqwerqwerqwerqwerqwer")
	updateString(trie, "120100", "qwerqwerqwerqwerqwerqwerqwerqwer")
	updateString(trie, "123456", "asdfasdfasdfasdfasdfasdfasdfasdf")
	root.Ver.Major++
	trie.Commit(db, root.Ver, false)
	root.Hash = trie.Hash()

	trie = New(root, db)
	_, _, err := trie.Get([]byte("120000"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie = New(root, db)
	_, _, err = trie.Get([]byte("120099"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie = New(root, db)
	_, _, err = trie.Get([]byte("123456"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie = New(root, db)
	err = trie.Update([]byte("120099"), []byte("zxcvzxcvzxcvzxcvzxcvzxcvzxcvzxcv"), nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie = New(root, db)
	err = trie.Update([]byte("123456"), nil, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	db.db.Delete(makeKey([]byte{3, 1, 3, 2, 3, 0, 3}, root.Ver))

	trie = New(root, db)
	_, _, err = trie.Get([]byte("120000"))
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}

	trie = New(root, db)
	_, _, err = trie.Get([]byte("120099"))
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}

	trie = New(root, db)
	_, _, err = trie.Get([]byte("123456"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	trie = New(root, db)
	err = trie.Update([]byte("120099"), []byte("zxcv"), nil)
	if _, ok := err.(*MissingNodeError); !ok {
		t.Errorf("Wrong error: %v", err)
	}
}

func TestInsert(t *testing.T) {
	trie := new(Trie)

	updateString(trie, "doe", "reindeer")
	updateString(trie, "dog", "puppy")
	updateString(trie, "dogglesworth", "cat")

	exp, _ := thor.ParseBytes32("6ca394ff9b13d6690a51dea30b1b5c43108e52944d30b9095227c49bae03ff8b")
	hash := trie.Hash()
	if hash != exp {
		t.Errorf("exp %v got %v", exp, hash)
	}

	trie = new(Trie)
	updateString(trie, "A", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	exp, _ = thor.ParseBytes32("e9d7f23f40cd82fe35f5a7a6778c3503f775f3623ba7a71fb335f0eee29dac8a")
	db := newMemDatabase()

	err := trie.Commit(db, Version{}, false)
	hash = trie.Hash()
	if err != nil {
		t.Fatalf("commit error: %v", err)
	}
	if hash != exp {
		t.Errorf("exp %v got %v", exp, hash)
	}
}

func TestGet(t *testing.T) {
	trie := new(Trie)
	updateString(trie, "doe", "reindeer")
	updateString(trie, "dog", "puppy")
	updateString(trie, "dogglesworth", "cat")
	db := newMemDatabase()

	for i := range 2 {
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
		trie.Commit(db, Version{Major: uint32(i)}, false)
	}
}

func TestDelete(t *testing.T) {
	trie := new(Trie)
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
	trie := new(Trie)

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
	db := newMemDatabase()
	trie := new(Trie)
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
	ver := Version{}
	if err := trie.Commit(db, ver, false); err != nil {
		t.Fatalf("commit error: %v", err)
	}
	exp := trie.Hash()

	// create a new trie on top of the database and check that lookups work.
	trie2 := New(Root{exp, ver}, db)

	for _, kv := range vals {
		if string(getString(trie2, kv.k)) != kv.v {
			t.Errorf("trie2 doesn't have %q => %q", kv.k, kv.v)
		}
	}
	ver.Major++
	if err := trie2.Commit(db, ver, false); err != nil {
		t.Fatalf("commit error: %v", err)
	}
	got := trie2.Hash()
	if got != exp {
		t.Errorf("root failure. expected %x got %x", exp, got)
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
	trie := new(Trie)
	trie.Update([]byte("key1"), []byte{99, 99, 99, 99}, nil)
	trie.Update([]byte("key2"), bytes.Repeat([]byte{1}, 32), nil)
	trie.Hash()
}

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
	for i := range size {
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
	db := newMemDatabase()
	root := Root{}
	tr := New(root, db)
	values := make(map[string]string) // tracks content of the trie

	for i, step := range rt {
		switch step.op {
		case opUpdate:
			tr.Update(step.key, step.value, nil)
			values[string(step.key)] = string(step.value)
		case opDelete:
			tr.Update(step.key, nil, nil)
			delete(values, string(step.key))
		case opGet:
			v, _, _ := tr.Get(step.key)
			want := values[string(step.key)]
			if string(v) != want {
				rt[i].err = fmt.Errorf("mismatch for key 0x%x, got 0x%x want 0x%x", step.key, v, want)
			}
		case opCommit:
			root.Ver.Major++
			rt[i].err = tr.Commit(db, root.Ver, false)
		case opHash:
			tr.Hash()
		case opReset:
			root.Ver.Major++
			if err := tr.Commit(db, root.Ver, false); err != nil {
				rt[i].err = err
				return false
			}
			root.Hash = tr.Hash()
			newtr := New(root, db)
			tr = newtr
		case opItercheckhash:
			checktr := new(Trie)
			it := NewIterator(tr.NodeIterator(nil, Version{}))
			for it.Next() {
				checktr.Update(it.Key, it.Value, nil)
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
	db := newMemDatabase()
	root := Root{}
	if commit {
		trie = New(root, db)
	}
	k := make([]byte, 32)
	for i := range benchElemCount {
		binary.LittleEndian.PutUint64(k, uint64(i))
		trie.Update(k, k, nil)
	}
	binary.LittleEndian.PutUint64(k, benchElemCount/2)
	if commit {
		root.Ver.Major++
		trie.Commit(db, root.Ver, false)
	}

	for b.Loop() {
		trie.Get(k)
	}
	b.StopTimer()
}

func benchUpdate(b *testing.B, e binary.ByteOrder) *Trie {
	trie := new(Trie)
	k := make([]byte, 32)
	for i := 0; b.Loop(); i++ {
		e.PutUint64(k, uint64(i))
		trie.Update(k, k, nil)
	}
	return trie
}

// Benchmarks the trie hashing. Since the trie caches the result of any operation,
// we cannot use b.N as the number of hashing rouns, since all rounds apart from
// the first one will be NOOP. As such, we'll use b.N as the number of account to
// insert into the trie before measuring the hashing.
func BenchmarkHash(b *testing.B) {
	// Make the random benchmark deterministic
	random := rand.New(rand.NewSource(0)) //#nosec G404

	// Create a realistic account trie to hash
	addresses := make([][20]byte, b.N)
	for i := range addresses {
		for j := range len(addresses[i]) {
			addresses[i][j] = byte(random.Intn(256))
		}
	}
	accounts := make([][]byte, len(addresses))
	for i := range accounts {
		var (
			nonce   = uint64(random.Int63())
			balance = new(big.Int).Rand(random, new(big.Int).Exp(common.Big2, common.Big256, nil))
			root    = emptyRoot
			code    = crypto.Keccak256(nil)
		)
		accounts[i], _ = rlp.EncodeToBytes([]any{nonce, balance, root, code})
	}
	// Insert the accounts into the trie and hash it
	trie := new(Trie)
	for i := range addresses {
		trie.Update(thor.Blake2b(addresses[i][:]).Bytes(), accounts[i], nil)
	}
	b.ResetTimer()
	b.ReportAllocs()
	trie.Hash()
}

func getString(trie *Trie, k string) []byte {
	val, _, err := trie.Get([]byte(k))
	if err != nil {
		panic(err)
	}
	return val
}

func updateString(trie *Trie, k, v string) {
	if err := trie.Update([]byte(k), []byte(v), nil); err != nil {
		panic(err)
	}
}

func deleteString(trie *Trie, k string) {
	if err := trie.Update([]byte(k), nil, nil); err != nil {
		panic(err)
	}
}

func TestExtended(t *testing.T) {
	db := newMemDatabase()
	ver := Version{}
	tr := New(Root{}, db)

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

	ver.Major++
	err := tr.Commit(db, ver, false)
	if err != nil {
		t.Errorf("commit failed %v", err)
	}
	root1 := tr.Hash()

	for _, v := range vals2 {
		tr.Update([]byte(v.k), []byte(v.v), thor.Blake2b([]byte(v.v)).Bytes())
	}
	ver.Major++
	err = tr.Commit(db, ver, false)
	if err != nil {
		t.Errorf("commit failed %v", err)
	}
	root2 := tr.Hash()

	tr1 := New(Root{root1, Version{Major: 1}}, db)
	for _, v := range vals1 {
		val, meta, _ := tr1.Get([]byte(v.k))
		if string(val) != v.v {
			t.Errorf("incorrect value for key '%v'", v.k)
		}
		if string(meta) != string(thor.Blake2b(val).Bytes()) {
			t.Errorf("incorrect value meta for key '%v'", v.k)
		}
	}

	tr2 := New(Root{root2, Version{Major: 2}}, db)
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

func TestCommitSkipHash(t *testing.T) {
	db := newMemDatabase()
	ver := Version{}
	tr := New(Root{}, db)
	n := uint32(100)
	for i := uint32(0); i < n; i++ {
		var k [4]byte
		binary.BigEndian.PutUint32(k[:], i)
		tr.Update(k[:], thor.Blake2b(k[:]).Bytes(), nil)
		ver.Major++
		tr.Commit(db, ver, true)
	}

	tr = New(Root{thor.BytesToBytes32([]byte{1}), ver}, db)
	for i := uint32(0); i < n; i++ {
		var k [4]byte
		binary.BigEndian.PutUint32(k[:], i)
		val, _, err := tr.Get(k[:])
		assert.Nil(t, err)
		assert.Equal(t, thor.Blake2b(k[:]).Bytes(), val)
	}
}

func TestFromRootNode(t *testing.T) {
	db := newMemDatabase()
	tr := New(Root{}, db)

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

	tr = FromRootNode(tr.RootNode(), db)

	for _, val := range vals {
		v, _, _ := tr.Get([]byte(val.k))
		if val.v != string(v) {
			t.Errorf("incorrect value for key '%v'", val.k)
		}
	}
}
