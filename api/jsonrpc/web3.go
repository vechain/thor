// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

type web3API struct{}

// web3_clientVersion
func (a *web3API) ClientVersion() string {
	return "thor"
}
