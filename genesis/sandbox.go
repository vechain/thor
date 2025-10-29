package genesis

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/thor"
)

func NewHayabusaSandbox() (*Genesis, *thor.ForkConfig) {
	hayabusaTP := uint32(0)
	builtInStargate := uint32(0)
	largeNo := (*HexOrDecimal256)(new(big.Int).SetBytes(hexutil.MustDecode("0xffffffffffffffffffffffffffffffffff")))
	fc := thor.ForkConfig{
		VIP191:    0,
		ETH_CONST: 0,
		BLOCKLIST: 0,
		ETH_IST:   0,
		VIP214:    0,
		FINALITY:  0,
		HAYABUSA:  0,
		GALACTICA: 0,
	}
	// Create accounts for all DevAccounts
	accounts := make([]Account, len(DevAccounts()))
	for i, devAcc := range DevAccounts() {
		accounts[i] = Account{
			Address: devAcc.Address,
			Balance: largeNo,
			Energy:  largeNo,
		}
	}

	gen := &CustomGenesis{
		LaunchTime: uint64(1526400000), // Default launch time 'Wed May 16 2018 00:00:00 GMT+0800 (CST)',
		GasLimit:   thor.InitialGasLimit,
		ExtraData:  "hayabusa solo stargate sandbox",
		Accounts:   accounts,
		Authority: []Authority{
			{
				MasterAddress:   DevAccounts()[0].Address,
				EndorsorAddress: DevAccounts()[0].Address,
				Identity:        thor.BytesToBytes32([]byte("Solo Block Signer")),
			},
		},
		Params: Params{
			ExecutorAddress: &DevAccounts()[0].Address,
		},
		ForkConfig: &fc,
		Config: &thor.Config{
			BlockInterval:       10,
			LowStakingPeriod:    12,
			MediumStakingPeriod: 30,
			HighStakingPeriod:   90,
			CooldownPeriod:      12,
			EpochLength:         6,
			HayabusaTP:          &hayabusaTP,
			BuiltInStargate:     &builtInStargate,
		},
	}
	customNet, err := NewCustomNet(gen)
	if err != nil {
		panic(err)
	}
	return customNet, &fc
}
