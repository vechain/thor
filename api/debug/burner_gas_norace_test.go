// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

//go:build !race

package debug

// burnerGas sizes TestReplayStopsWithContext's gas burner. The race detector
// slows execution by an order of magnitude, so it gets less gas for a replay of
// similar wall time.
const burnerGas = 10_000_000
