package thor

import (
	"errors"
	"fmt"
	"math"
)

// ForkConfig config for a fork.
type ForkConfig struct {
	FixTransferLog uint32
	VIP191         uint32
}

func (fc ForkConfig) String() string {
	return fmt.Sprintf("FTRL: #%v", fc.VIP191)
}

// NoFork a special config without any forks.
var NoFork = ForkConfig{
	FixTransferLog: math.MaxUint32,
	VIP191:         math.MaxUint32,
}

// for well-known networks
var forkConfigs = map[Bytes32]ForkConfig{
	// mainnet
	MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"): {
		FixTransferLog: 1072000,
		VIP191:         math.MaxUint32,
	},
	// testnet
	MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127"): {
		FixTransferLog: 1080000,
		VIP191:         math.MaxUint32,
	},
}

// GetForkConfig get fork config for given genesis ID.
func GetForkConfig(genesisID Bytes32) ForkConfig {
	return forkConfigs[genesisID]
}

// SetCustomNetForkConfig set the fork config for the given genesis ID.
func SetCustomNetForkConfig(genesisID Bytes32, f ForkConfig) error {
	if _, ok := forkConfigs[genesisID]; ok {
		return errors.New("Can not overwrite fork config")
	}
	forkConfigs[genesisID] = f
	return nil
}
