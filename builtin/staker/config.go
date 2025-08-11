package staker

import (
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

type Config struct {
	EpochLength    uint32
	CooldownPeriod uint32
	LowStakingPeriod    uint32
	MediumStakingPeriod uint32
	HighStakingPeriod   uint32
}

func (c Config) DecodeSlots(slots []thor.Bytes32) error {
	if len(slots) != 1 {
		return errors.New("invalid number of slots for Config, expected 1 slot")
	}

	bytes := slots[0].Bytes()
	if len(bytes) < 20 {
		return errors.New("invalid slot length for Config, expected at least 20 bytes")
	}

	c.EpochLength = binary.BigEndian.Uint32(bytes[:4])
	c.CooldownPeriod = binary.BigEndian.Uint32(bytes[4:8])
	c.LowStakingPeriod = binary.BigEndian.Uint32(bytes[8:12])
	c.MediumStakingPeriod = binary.BigEndian.Uint32(bytes[12:16])
	c.HighStakingPeriod = binary.BigEndian.Uint32(bytes[16:20])

	return nil
}

func (c Config) EncodeSlots() []thor.Bytes32 {
	var bytes []byte

	binary.BigEndian.PutUint32(bytes, c.EpochLength)
	binary.BigEndian.AppendUint32(bytes, c.CooldownPeriod)
	binary.BigEndian.AppendUint32(bytes, c.LowStakingPeriod)
	binary.BigEndian.AppendUint32(bytes, c.MediumStakingPeriod)
	binary.BigEndian.AppendUint32(bytes, c.HighStakingPeriod)

	var slot thor.Bytes32
	copy(slot[:], bytes)

	return []thor.Bytes32{slot}
}

func (c Config) UsedSlots() int {
	return 1
}

var _ solidity.ComplexValue[*Config] = (*Config)(nil)
