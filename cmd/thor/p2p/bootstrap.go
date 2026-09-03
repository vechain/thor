// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2p

import (
	"github.com/vechain/thor/v2/p2p/discover"
)

const remoteDiscoveryNodesList = "https://vechain.github.io/bootstraps/node.list"

var fallbackDiscoveryNodes = []*discover.Node{
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
