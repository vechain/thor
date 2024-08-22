// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package main

import (
	"os"

	"github.com/vechain/thor/v2/cmd/disco/runtime"
)

func main() {
	runtime.Start(os.Args)
}
