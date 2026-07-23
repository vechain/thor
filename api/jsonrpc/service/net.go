// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package service

import "strconv"

// Net implements the net_* JSON-RPC namespace.
type Net struct{ b *Backend }

// NewNet creates a Net service backed by b.
func NewNet(b *Backend) *Net { return &Net{b: b} }

// Version implements net_version.
func (a *Net) Version() string {
	return strconv.FormatUint(a.b.repo.ChainID(), 10)
}
