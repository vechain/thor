// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2p

import (
	"github.com/ethereum/go-ethereum/p2p/discover"
)

const remoteDiscoveryNodesList = "https://vechain.github.io/bootstraps/node.list"

var fallbackDiscoveryNodes = []*discover.Node{
	discover.MustParseNode(
		"enode://797fdd968592ca3b59a143f1aa2f152913499d4bb469f2bd5b62dfb1257707b4cb0686563fe144ee2088b1cc4f174bd72df51dbeb7ec1c5b6a8d8599c756f38b@107.150.112.22:55555",
	),
	discover.MustParseNode(
		"enode://3eae6740af6180bb015309f7a07ff7405d6f1f9f1e5a9f2fabbd36b0c00b862521e63ff23573ffdb9035f2237c26513cb9f02454f9ada993e60b99ffc187bb54@107.150.112.21:55555",
	),
	discover.MustParseNode(
		"enode://12e90ad91b7c9abe1788cdd7804b1ea48f2983a99320c62a6aaa9ee71148ec9eb0a30ccb1c66acc46d27adcb8e636f141366d1894e631b93dfdfd416309be929@152.32.151.143:55555",
	),
	discover.MustParseNode(
		"enode://d4cca8cb2ac7ff8cbe768a0d901bca8a04e2e7ad7bf0d03b576d779ca442dde107456702be695993a5cda3be96329043b742aad770c4bfecdf4d1cfc104dc04b@128.1.41.27:55555",
	),
	discover.MustParseNode(
		"enode://4117bb1386e67454a61ca832bf353673cbed85d7e4e2a6a5d8e8c899a13c786380f92bc77bc0f54870d5076f0eee4e0c71d9405113a6c3de4d76f61282985449@165.154.164.48:55555",
	),
	discover.MustParseNode(
		"enode://1094cf28ab5f0255a3923ac094d0168ce884a9fa5f3998b1844986b4a2b1eac52fcccd8f2916be9b8b0f7798147ee5592ec3c83518925fac50f812577515d6ad@152.32.198.254:55555",
	),
	discover.MustParseNode(
		"enode://f0e93c6be07f15427a017d158498c7ca9397541d24b4efbd1bb368155f6de1ae07a9a2da81a7f116e60e86c26eb0c70f2cae4516c3b5b6cfe2e5f522252665cc@54.78.133.203:55555",
	),
	discover.MustParseNode(
		"enode://10a52cd99873ae668908215a4cbfa814e9091c145150ff12a8ad5b017b470233d69bff44d0c418895f29ffab2089b82ea8a77a8d209dd6ad3b5cc6122932e1ba@18.159.9.93:55555",
	),
	discover.MustParseNode(
		"enode://06d58ccb89f40413f46530dabcf0bb19fcca742182cfa5533b93a429ff0c29c0f9160fd9d88a3247b0cf2a82c33bccdff8ad0aaf1bd0401128d7d614fca37c57@18.143.229.38:55555",
	),
	discover.MustParseNode(
		"enode://72e94d9b6ba54c9579f53cf58bfb2de67c4782db723d978a85309bf69b0143265256cca3bb17d589fd4fbd3d73f01274320e485ace64ec2e5f3eda2857c6bff1@54.251.241.170:55555",
	),
	discover.MustParseNode(
		"enode://f83949d1b3079de4d103729d34ee1b051d0b2bc39c0ea9513350119187c38c9db0ed65ad5ca588766ed376b5674e575beab64b7daf7cf658af100df4e35bd196@34.194.173.111:55555",
	),
	discover.MustParseNode(
		"enode://54c60d4bac84e503dbf3534629df31b9f0e7331985052d58830b27344e765bf42290d8d7d8adc41cd556bd64555343b0a698a202e7446599de5e103d98cd33cd@54.145.187.159:55555",
	),
}
