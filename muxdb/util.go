// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// +build !darwin

package muxdb

import (
	"fmt"
	"os"
	"runtime"
)

func disablePageCache(f *os.File) {
	fmt.Fprintln(os.Stderr, "disablePageCache unsupported for OS", runtime.GOOS)
}
