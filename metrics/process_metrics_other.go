// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

//go:build !linux

package metrics

// registerIOCollector is a no-op on non-Linux platforms.
// Process metrics collection requires the /proc filesystem which is only available on Linux.
func registerIOCollector() {
	// No-op: /proc filesystem is not available on this platform
}
