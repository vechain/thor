// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
)

type errorPutter struct {
	putErr    error
	deleteErr error
}

func (e *errorPutter) Put(key, value []byte) error {
	if e.putErr != nil {
		return e.putErr
	}
	return nil
}

func (e *errorPutter) Delete(key []byte) error {
	if e.deleteErr != nil {
		return e.deleteErr
	}
	return nil
}

func TestSaveRLP_Errors(t *testing.T) {
	key := []byte("key")
	val := &block.Header{}

	t.Run("Put error", func(t *testing.T) {
		putter := &errorPutter{putErr: errors.New("put error")}
		err := saveRLP(putter, key, val)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "put error")
	})

	t.Run("RLP encode error", func(t *testing.T) {
		putter := &errorPutter{}
		ch := make(chan int)
		err := saveRLP(putter, key, ch)
		assert.Error(t, err)
	})
}

func TestIndexChainHead_Errors(t *testing.T) {
	header := &block.Header{}

	t.Run("Delete error", func(t *testing.T) {
		putter := &errorPutter{deleteErr: errors.New("delete error")}
		err := indexChainHead(putter, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delete error")
	})

	t.Run("Put error", func(t *testing.T) {
		putter := &errorPutter{putErr: errors.New("put error")}
		err := indexChainHead(putter, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "put error")
	})
}
