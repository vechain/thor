// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import "strconv"

type netAPI struct{ b *backend }

// net_version
func (a *netAPI) Version() string {
	return strconv.FormatUint(a.b.repo.ChainID(), 10)
}
