// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"fmt"
	"math"
	"strings"
)

// ForkConfig config for a fork.
type ForkConfig struct {
	VIP191    uint32
	ETH_CONST uint32
	BLOCKLIST uint32
	ETH_IST   uint32
	VIP214    uint32
	FINALITY  uint32
}

func (fc ForkConfig) String() string {
	var strs []string
	push := func(name string, blockNum uint32) {
		if blockNum != math.MaxUint32 {
			strs = append(strs, fmt.Sprintf("%v: #%v", name, blockNum))
		}
	}

	push("VIP191", fc.VIP191)
	push("ETH_CONST", fc.ETH_CONST)
	push("BLOCKLIST", fc.BLOCKLIST)
	push("ETH_IST", fc.ETH_IST)
	push("VIP214", fc.VIP214)
	push("FINALITY", fc.FINALITY)

	return strings.Join(strs, ", ")
}

// NoFork a special config without any forks.
var NoFork = ForkConfig{
	VIP191:    math.MaxUint32,
	ETH_CONST: math.MaxUint32,
	BLOCKLIST: math.MaxUint32,
	ETH_IST:   math.MaxUint32,
	VIP214:    math.MaxUint32,
	FINALITY:  math.MaxUint32,
}

// for well-known networks
var forkConfigs = map[Bytes32]ForkConfig{
	// mainnet
	MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"): {
		VIP191:    3337300,
		ETH_CONST: 3337300,
		BLOCKLIST: 4817300,
		ETH_IST:   9254300,
		VIP214:    10653500, // ~ Tue Nov 16 2021 08:00:00 GMT
		FINALITY:  math.MaxUint32,
	},
	// testnet
	MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127"): {
		VIP191:    2898800,
		ETH_CONST: 3192500,
		BLOCKLIST: math.MaxUint32,
		ETH_IST:   9146700,
		VIP214:    10606800,
		FINALITY:  13086360, // ~ Fri, 19 Aug 2022 08:00:00 GMT
	},
}

// GetForkConfig get fork config for given genesis ID.
func GetForkConfig(genesisID Bytes32) ForkConfig {
	return forkConfigs[genesisID]
}
