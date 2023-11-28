// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"github.com/ethereum/go-ethereum/p2p/discover"
)

const remoteBootstrapList = "https://vechain.github.io/bootstraps/node.list"

var fallbackBootstrapNodes = []*discover.Node{
	discover.MustParseNode("enode://797fdd968592ca3b59a143f1aa2f152913499d4bb469f2bd5b62dfb1257707b4cb0686563fe144ee2088b1cc4f174bd72df51dbeb7ec1c5b6a8d8599c756f38b@107.150.112.22:55555"),
	discover.MustParseNode("enode://3eae6740af6180bb015309f7a07ff7405d6f1f9f1e5a9f2fabbd36b0c00b862521e63ff23573ffdb9035f2237c26513cb9f02454f9ada993e60b99ffc187bb54@107.150.112.21:55555"),
	discover.MustParseNode("enode://12e90ad91b7c9abe1788cdd7804b1ea48f2983a99320c62a6aaa9ee71148ec9eb0a30ccb1c66acc46d27adcb8e636f141366d1894e631b93dfdfd416309be929@152.32.151.143:55555"),
	discover.MustParseNode("enode://d4cca8cb2ac7ff8cbe768a0d901bca8a04e2e7ad7bf0d03b576d779ca442dde107456702be695993a5cda3be96329043b742aad770c4bfecdf4d1cfc104dc04b@128.1.41.27:55555"),
	discover.MustParseNode("enode://4117bb1386e67454a61ca832bf353673cbed85d7e4e2a6a5d8e8c899a13c786380f92bc77bc0f54870d5076f0eee4e0c71d9405113a6c3de4d76f61282985449@165.154.164.48:55555"),
	discover.MustParseNode("enode://1094cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@152.32.198.254:55555"),
}
