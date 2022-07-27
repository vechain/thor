// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

import (
	"errors"
	"reflect"
	"testing"
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
func (m mem) IsNotFound(err error) bool {
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
