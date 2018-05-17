// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import "github.com/ethereum/go-ethereum/p2p"

// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	p2p.Protocol

	DiscTopic string
}
