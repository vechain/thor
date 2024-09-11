// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mem map[string]string

func (m mem) Get(k []byte) ([]byte, error) {
	if v, ok := m[string(k)]; ok {
		return []byte(v), nil
	}
	return nil, errors.New("not found")
}

func (m mem) Has(k []byte) (bool, error) {
	_, ok := m[string(k)]
	return ok, nil
}

func (m mem) Put(k, v []byte) error {
	m[string(k)] = string(v)
	return nil
}

func (m mem) Delete(k []byte) error {
	delete(m, string(k))
	return nil
}
func (m mem) IsNotFound(error) bool {
	return true
}

func TestBucket_GetterGet(t *testing.T) {
	m := mem{"k1": "v1", "k2": "v2"}

	tests := []struct {
		b    Bucket
		key  string
		want string
	}{
		{Bucket(""), "k1", "v1"},
		{Bucket(""), "k2", "v2"},
		{Bucket("k"), "k1", ""},
		{Bucket("k"), "1", "v1"},
		{Bucket("k"), "2", "v2"},
		{Bucket("k1"), "", "v1"},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got, _ := tt.b.NewGetter(m).Get([]byte(tt.key)); !reflect.DeepEqual(string(got), tt.want) {
				t.Errorf("Bucket.NewGetter.Get = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestBucket_GetterHas(t *testing.T) {
	m := mem{"k1": "v1", "k2": "v2"}

	tests := []struct {
		b    Bucket
		key  string
		want bool
	}{
		{Bucket(""), "k1", true},
		{Bucket(""), "k2", true},
		{Bucket("k"), "k1", false},
		{Bucket("k"), "1", true},
		{Bucket("k"), "2", true},
		{Bucket("k1"), "", true},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got, _ := tt.b.NewGetter(m).Has([]byte(tt.key)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Bucket.NewGetter.Has = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPutter(t *testing.T) {
	m := mem{"k1": "v1", "k2": "v2"}

	tests := []struct {
		b    Bucket
		key  string
		want string
	}{
		{Bucket(""), "k1", "v1"},
		{Bucket(""), "k2", "v2"},
		{Bucket("k"), "k1", ""},
		{Bucket("k"), "1", "v1"},
		{Bucket("k"), "2", "v2"},
		{Bucket("k1"), "", "v1"},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if err := tt.b.NewPutter(m).Put([]byte(tt.key), []byte(tt.want)); err != nil {
				t.Errorf("Bucket.NewPutter.Put failed")
			}
		})
	}

	// test delete too
	err := tests[0].b.NewPutter(m).Delete([]byte("k1"))
	assert.Nil(t, err)
}

// Mock DummyStore Implementations
type DummyStore struct {
	data map[string][]byte
}

func NewDummyStore() *DummyStore {
	return &DummyStore{
		data: make(map[string][]byte),
	}
}

func (s *DummyStore) Get(key []byte) ([]byte, error) {
	val, exists := s.data[string(key)]
	if !exists {
		return nil, errors.New("key not found")
	}

	return val, nil
}

func (s *DummyStore) Has(key []byte) (bool, error) {
	_, exists := s.data[string(key)]
	return exists, nil
}

func (s *DummyStore) IsNotFound(err error) bool {
	return err.Error() == "key not found"
}

func (s *DummyStore) Put(key, val []byte) error {
	s.data[string(key)] = val
	return nil
}

func (s *DummyStore) Delete(key []byte) error {
	delete(s.data, string(key))
	return nil
}

func (s *DummyStore) DeleteRange(_ context.Context, r Range) error {
	for k := range s.data {
		if k >= string(r.Start) && k < string(r.Limit) {
			delete(s.data, k)
		}
	}
	return nil
}

func (s *DummyStore) Iterate(_ Range) Iterator {
	return &DummyIterator{}
}

func (s *DummyStore) Bulk() Bulk {
	return &DummyBulk{}
}

func (s *DummyStore) Snapshot() Snapshot {
	return &DummySnapshot{}
}

// Dummy Bulk Implementation
type DummyBulk struct{}

func (db *DummyBulk) Put(_, _ []byte) error {
	return nil
}

func (db *DummyBulk) Delete(_ []byte) error {
	return nil
}

func (db *DummyBulk) EnableAutoFlush() {
}

func (db *DummyBulk) Write() error {
	return nil
}

// Dummy Iterator Implementation
type DummyIterator struct{}

func (di *DummyIterator) First() bool {
	return true
}

func (di *DummyIterator) Last() bool {
	return true
}

func (di *DummyIterator) Next() bool {
	return false
}

func (di *DummyIterator) Prev() bool {
	return false
}

func (di *DummyIterator) Key() []byte {
	return []byte{}
}

func (di *DummyIterator) Value() []byte {
	return []byte{}
}

func (di *DummyIterator) Release() {
}

func (di *DummyIterator) Error() error {
	return nil
}

// Dummy Snapshot implementation
type DummySnapshot struct{}

func (ds *DummySnapshot) Get(_ []byte) ([]byte, error) {
	return nil, errors.New("key not found")
}

func (ds *DummySnapshot) Has(key []byte) (bool, error) {
	return false, nil
}

func (ds *DummySnapshot) IsNotFound(err error) bool {
	return err.Error() == "key not found"
}

func (ds *DummySnapshot) Release() {
}

func TestNewStore(t *testing.T) {
	bucket := Bucket("")

	DummyStore := NewDummyStore()
	DummyStore.Put([]byte("k1"), []byte("v1"))
	DummyStore.Put([]byte("k2"), []byte("v2"))

	store := bucket.NewStore(DummyStore)

	store.Bulk()

	myRange := Range{
		Start: []byte("k1"),
		Limit: []byte("k1"),
	}

	interateRes := store.Iterate(myRange)
	assert.NotNil(t, interateRes)

	snapshotRes := store.Snapshot()
	assert.NotNil(t, snapshotRes)

	isNotFoundRes := store.IsNotFound(errors.New("key not found"))
	assert.NotNil(t, isNotFoundRes)

	err := store.Bulk().Write()
	assert.Nil(t, err)

	err = store.DeleteRange(context.Background(), myRange)
	assert.Nil(t, err)
}
