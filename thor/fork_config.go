// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"fmt"
	"math"
	"strings"
)

// nolint: revive
// ForkConfig config for a fork.
type ForkConfig struct {
	VIP191    uint32
	ETH_CONST uint32
	BLOCKLIST uint32
	ETH_IST   uint32
	VIP214    uint32
	FINALITY  uint32
	HAYABUSA  uint32 // Start of the Hayabusa Transition Period - PoA is still active until the transition period is over and 2/3 of the MBP have entered the PoS queue
	GALACTICA uint32
}

func (fc *ForkConfig) String() string {
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
	push("GALACTICA", fc.GALACTICA)
	push("HAYABUSA", fc.HAYABUSA)

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
	GALACTICA: math.MaxUint32,
	HAYABUSA:  math.MaxUint32,
}

// SoloFork is used to define the solo fork config.
var SoloFork = ForkConfig{
	VIP191:    0,
	ETH_CONST: 0,
	BLOCKLIST: 0,
	ETH_IST:   0,
	VIP214:    0,
	FINALITY:  0,
	// Any subsequent fork should be started from block 1.
	GALACTICA: 1,
	HAYABUSA:  1,
}

// for well-known networks
var forkConfigs = map[Bytes32]*ForkConfig{
	// mainnet
	MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"): {
		VIP191:    3_337_300,
		ETH_CONST: 3_337_300,
		BLOCKLIST: 4_817_300,
		ETH_IST:   9_254_300,
		VIP214:    10_653_500,
		FINALITY:  13_815_000,
		GALACTICA: 22_084_200, // ~ Tue, 01 Jul 2025 12:00:00 UTC
		HAYABUSA:  math.MaxUint32,
	},
	// testnet
	MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127"): {
		VIP191:    2_898_800,
		ETH_CONST: 3_192_500,
		BLOCKLIST: math.MaxUint32,
		ETH_IST:   9_146_700,
		VIP214:    10_606_800,
		FINALITY:  13_086_360,
		GALACTICA: 21_770_500, // ~ Tue, 20 May 2025 12:00:00 UTC
		HAYABUSA:  23_161_140,
	},
}

// GetForkConfig get fork config for the given genesis ID.
// Only works for the well-known networks.Custom network will get nil.
func GetForkConfig(genesisID Bytes32) *ForkConfig {
	return forkConfigs[genesisID]
}
