// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package convert holds converters shared across api handler packages.
package convert

// MapSlice maps xs through fn, preserving nil.
func MapSlice[A, B any](xs []A, fn func(A) B) []B {
	if xs == nil {
		return nil
	}
	ys := make([]B, len(xs))
	for i, x := range xs {
		ys[i] = fn(x)
	}
	return ys
}
