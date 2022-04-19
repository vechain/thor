// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"errors"
)

var errConflictWithFinalized = errors.New("block conflict with committeed")

func IsConflictWithFinalized(err error) bool {
	return err == errConflictWithFinalized
}
