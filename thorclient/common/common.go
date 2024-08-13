// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package common

import (
	"fmt"

	"github.com/vechain/thor/v2/thor"
)

var (
	NotFoundErr     = fmt.Errorf("not found")
	Not200StatusErr = fmt.Errorf("not 200 status code")
)

type TxSendResult struct {
	ID *thor.Bytes32
}
