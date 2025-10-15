// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

// Config is the configurable parameters of the thor. Most of the parameters will have default values and
// will be 'locked' for production networks. For testing purposes or custom networks, the parameters can be updated.

var (
	blockInterval              uint64 = 10       // 10 seconds
	epochLength                uint32 = 180      // 180 blocks, 30 minutes
	seederInterval             uint32 = 8640     // 8640 blocks, 1 day
	validatorEvictionThreshold uint32 = 7 * 8640 // 7 days
	evictionCheckInterval      uint32 = 8640 * 3 // 3 days

	// Staker parameters
	lowStakingPeriod    uint32 = 8640 * 7  // 7 Days
	mediumStakingPeriod uint32 = 8640 * 15 // 15 Days
	highStakingPeriod   uint32 = 8640 * 30 // 30 Days
	cooldownPeriod      uint32 = 8640      // 8640 blocks, 1 day
	hayabusaTP          uint32 = 8640 * 7  // 7 days

	locked bool
)

type Config struct {
	BlockInterval              uint64 `json:"blockInterval"`              // time interval between two consecutive blocks.
	EpochLength                uint32 `json:"epochLength"`                // number of blocks per epoch, also the number of blocks between two checkpoints.
	SeederInterval             uint32 `json:"seederInterval"`             // blocks between two scheduler seeder epochs.
	ValidatorEvictionThreshold uint32 `json:"validatorEvictionThreshold"` // the number of blocks after which offline validator will be evicted from the leader group (7 days)
	EvictionCheckInterval      uint32 `json:"evictionCheckInterval"`      // blocks between two eviction function executions

	// staker parameters
	LowStakingPeriod    uint32  `json:"lowStakingPeriod"`
	MediumStakingPeriod uint32  `json:"mediumStakingPeriod"`
	HighStakingPeriod   uint32  `json:"highStakingPeriod"`
	CooldownPeriod      uint32  `json:"cooldownPeriod"`
	HayabusaTP          *uint32 `json:"hayabusaTP"`
}

// SetConfig sets the config.
// If the config is not set, the default values will be used.
// If the config is locked, will panic.
func SetConfig(cfg Config) {
	if locked {
		panic("config is locked, cannot be set")
	}

	if cfg.BlockInterval != 0 {
		blockInterval = cfg.BlockInterval
	}

	if cfg.EpochLength != 0 {
		epochLength = cfg.EpochLength
	}

	if cfg.SeederInterval != 0 {
		seederInterval = cfg.SeederInterval
	}

	if cfg.ValidatorEvictionThreshold != 0 {
		validatorEvictionThreshold = cfg.ValidatorEvictionThreshold
	}

	if cfg.LowStakingPeriod != 0 {
		lowStakingPeriod = cfg.LowStakingPeriod
	}

	if cfg.MediumStakingPeriod != 0 {
		mediumStakingPeriod = cfg.MediumStakingPeriod
	}

	if cfg.HighStakingPeriod != 0 {
		highStakingPeriod = cfg.HighStakingPeriod
	}

	if cfg.CooldownPeriod != 0 {
		cooldownPeriod = cfg.CooldownPeriod
	}

	if cfg.HayabusaTP != nil {
		hayabusaTP = *cfg.HayabusaTP
	}

	if cfg.EvictionCheckInterval != 0 {
		evictionCheckInterval = cfg.EvictionCheckInterval
	}
}

// LockConfig locks the config, preventing any further changes.
// Required for mainnet and testnet.
func LockConfig() { locked = true }

// default value is 10 seconds
func BlockInterval() uint64 { return blockInterval }

// default value is 180 blocks, 30 minutes
func EpochLength() uint32 { return epochLength }

// default value is 8640 blocks, 1 day
func SeederInterval() uint32 { return seederInterval }

// default value is 7 days
func ValidatorEvictionThreshold() uint32 { return validatorEvictionThreshold }

// default value is 7 days
func LowStakingPeriod() uint32 { return lowStakingPeriod }

// default value is 15 days
func MediumStakingPeriod() uint32 { return mediumStakingPeriod }

// default value is 30 days
func HighStakingPeriod() uint32 { return highStakingPeriod }

// default value is 1 day
func CooldownPeriod() uint32 { return cooldownPeriod }

// default value is 7 days
func HayabusaTP() uint32 { return hayabusaTP }

// default value is 3 days
func EvictionCheckInterval() uint32 { return evictionCheckInterval }
