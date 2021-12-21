// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package optimizer

import (
	"encoding/json"

	"github.com/vechain/thor/kv"
)

type status struct {
	Base      uint32
	PruneBase uint32
}

func (s *status) Load(getter kv.Getter) error {
	data, err := getter.Get([]byte(statusKey))
	if err != nil && !getter.IsNotFound(err) {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, s)
}

func (s *status) Save(putter kv.Putter) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return putter.Put([]byte(statusKey), data)
}
