// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// +build darwin

package muxdb

import (
	"fmt"
	"os"
	"syscall"
)

func disablePageCache(f *os.File) {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_NOCACHE, 1)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "failed to set F_NOCACHE: %v\n", errno)
	}
}
