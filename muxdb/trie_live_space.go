// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"errors"
	"sync/atomic"

	"github.com/vechain/thor/kv"
)

const trieLiveSpaceKey = "trie-live-space"

// trieLiveSpace manages space for live trie nodes.
type trieLiveSpace struct {
	config kv.Store
	active atomic.Value
}

func newTrieLiveSpace(config kv.Store) (*trieLiveSpace, error) {
	val, err := config.Get([]byte(trieLiveSpaceKey))
	if err != nil && !config.IsNotFound(err) {
		return nil, err
	}

	space := &trieLiveSpace{config: config}
	if valLen := len(val); valLen == 1 {
		space.active.Store(val[0])
	} else if valLen == 0 {
		space.active.Store(trieSpaceA)
	} else {
		return nil, errors.New("invalid value of " + trieLiveSpaceKey)
	}
	return space, nil
}

func (s *trieLiveSpace) Switch() error {
	newActive := s.Stale()
	if err := s.config.Put([]byte(trieLiveSpaceKey), []byte{newActive}); err != nil {
		return err
	}
	s.active.Store(newActive)
	return nil
}

func (s *trieLiveSpace) Active() byte {
	return s.active.Load().(byte)
}

func (s *trieLiveSpace) Stale() byte {
	if s.Active() == trieSpaceA {
		return trieSpaceB
	}
	return trieSpaceA
}
