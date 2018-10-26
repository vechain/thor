package thor

import (
	"fmt"
	"math"
)

// ForkConfig config for a fork.
type ForkConfig struct {
	FixTransferLog uint32
}

func (fc ForkConfig) String() string {
	return fmt.Sprintf("FTRL: #%v", fc.FixTransferLog)
}

// NoFork a special config without any forks.
var NoFork = ForkConfig{
	FixTransferLog: math.MaxUint32,
}

// for well-known networks
var forkConfigs = map[Bytes32]ForkConfig{
	// mainnet
	MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"): {
		FixTransferLog: 1072000,
	},
	// testnet
	MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127"): {
		FixTransferLog: 1080000,
	},
}

// GetForkConfig get fork config for given genesis ID.
func GetForkConfig(genesisID Bytes32) ForkConfig {
	return forkConfigs[genesisID]
}
