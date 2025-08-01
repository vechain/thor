// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"bytes"
	"reflect"
	"sync"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

type Key interface {
	Bytes() []byte
}

// Mapping is a key/value storage abstraction for built-in contracts, similar to the mapping in Solidity.
// It DOES NOT (TBD) allow for direct access to values if declared in the same `pos` in the built-in contract.
type Mapping[K Key, V any] struct {
	context *Context
	basePos thor.Bytes32
}

func NewMapping[K Key, V any](context *Context, pos thor.Bytes32) *Mapping[K, V] {
	return &Mapping[K, V]{context: context, basePos: pos}
}

func (m *Mapping[K, V]) Get(key K) (value V, err error) {
	// compute the storage slot from key + base position
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())

	err = m.context.state.DecodeStorage(m.context.address, position, func(raw []byte) error {
		if len(raw) == 0 {
			// on missing-key, allocate a fresh pointer if V is a pointer type
			typ := reflect.TypeOf(&value).Elem()
			if typ.Kind() == reflect.Ptr {
				value = reflect.New(typ.Elem()).Interface().(V)
			}
			return nil
		}

		// charge gas per 32-byte word
		slots := (uint64(len(raw)) + 31) / 32
		m.context.UseGas(slots * thor.SloadGas)

		// DECODE via pooled reader to avoid bytes.Reader alloc
		return decodeValue(raw, &value)
	})
	return
}

func (m *Mapping[K, V]) Set(key K, value V, newValue bool) error {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())

	return m.context.state.EncodeStorage(m.context.address, position, func() ([]byte, error) {
		// ENCODE via pooled buffer to cut down on allocations
		buf, err := encodeValue(value)
		if err != nil {
			return nil, err
		}

		// charge gas per 32-byte word
		slots := (uint64(len(buf)) + 31) / 32
		if newValue {
			m.context.UseGas(slots * thor.SstoreSetGas)
		} else {
			m.context.UseGas(slots * thor.SstoreResetGas)
		}

		return buf, nil
	})
}

// ---------- RLP pooling helpers ----------

var encodeBufPool = sync.Pool{
	New: func() interface{} { return new(bytes.Buffer) },
}

// encodeValue reuses a bytes.Buffer from the pool and copies out the result.
// avoids repeated buffer allocations in Set
func encodeValue(v interface{}) ([]byte, error) {
	buf := encodeBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer encodeBufPool.Put(buf)

	if err := rlp.Encode(buf, v); err != nil {
		return nil, err
	}
	// copy to independent slice for storage safety
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

var readerPool = sync.Pool{
	New: func() interface{} { return new(bytes.Reader) },
}

// decodeValue reuses a bytes.Reader from the pool.
// avoids allocating a new reader on each Get
func decodeValue(raw []byte, out interface{}) error {
	rdr := readerPool.Get().(*bytes.Reader)
	rdr.Reset(raw)
	defer readerPool.Put(rdr)

	return rlp.NewStream(rdr, 0).Decode(out)
}
