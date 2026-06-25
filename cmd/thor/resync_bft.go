// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"fmt"

	"gopkg.in/cheggaaa/pb.v1"

	"github.com/vechain/thor/v2/bft"
)

// resyncBFT runs the engine's resync pass, rendering a progress bar from the callbacks.
func resyncBFT(bftEngine *bft.Engine) error {
	var bar *pb.ProgressBar
	defer func() {
		if bar != nil {
			bar.NotPrint = true
		}
	}()

	return bftEngine.Resync(func(done, total uint32) {
		if bar == nil {
			fmt.Println(">> Resyncing bft quality <<")
			bar = pb.New64(int64(total)).SetMaxWidth(90).Start()
		}
		bar.Set64(int64(done))
		if done == total {
			bar.Finish()
		}
	})
}
