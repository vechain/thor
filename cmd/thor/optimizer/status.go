// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package optimizer

import (
	"encoding/json"

	"github.com/vechain/thor/muxdb"
)

type status struct {
	Base        uint32
	AccountBase uint32
}

func (s *status) Load(db *muxdb.MuxDB) error {
	data, err := db.NewStore(propsStoreName).Get([]byte(statusKey))
	if err != nil && !db.IsNotFound(err) {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, s)
}

func (s *status) Save(db *muxdb.MuxDB) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.NewStore(propsStoreName).Put([]byte(statusKey), data)
}
