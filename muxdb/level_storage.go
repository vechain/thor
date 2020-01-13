// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// +build !darwin

package muxdb

import (
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func openLevelFileStorage(path string, readOnly, disablePageCache bool) (storage.Storage, error) {
	return storage.OpenFile(path, readOnly)
}
