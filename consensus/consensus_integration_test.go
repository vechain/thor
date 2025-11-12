package consensus_test

import (
	"crypto/rand"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

func TestConsensus_ReplayStopsEnergyAtHardfork_Matrix(t *testing.T) {
	cases := []struct {
		name       string
		hayabusa   uint32
		expectStop bool
	}{
		{"replay stops at hardfork", 2, true},
		{"replay does not stop without fork", math.MaxUint32, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := thor.SoloFork
			cfg.HAYABUSA = tc.hayabusa
			hayabusaTP := uint32(1)
			thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

			chain, err := testchain.NewWithFork(&cfg, 2)
			assert.NoError(t, err)

			assert.NoError(t, chain.MintBlock())
			assert.NoError(t, chain.MintBlock())

			best := chain.Repo().BestBlockSummary()
			c := consensus.New(chain.Repo(), chain.Stater(), &cfg)

			_, err = c.NewRuntimeForReplay(best.Header, false)
			assert.NoError(t, err)

			st := chain.Stater().NewState(best.Root())
			stop, err := builtin.Energy.Native(st, best.Header.Timestamp()).GetEnergyGrowthStopTime()
			assert.NoError(t, err)
			if tc.expectStop {
				assert.Equal(t, best.Header.Timestamp(), stop)
			} else {
				assert.Equal(t, uint64(math.MaxUint64), stop)
			}
		})
	}
}

func TestValidateBlockHeaderWithBadBaseFee(t *testing.T) {
	forkConfig := testchain.DefaultForkConfig
	forkConfig.GALACTICA = 1
	forkConfig.VIP214 = 2

	chain, err := testchain.NewWithFork(&forkConfig, 180)
	assert.NoError(t, err)

	con := consensus.New(chain.Repo(), chain.Stater(), &forkConfig)

	best, err := chain.BestBlock()
	assert.NoError(t, err)

	var sig [65]byte
	rand.Read(sig[:])
	newBlock := new(block.Builder).
		ParentID(best.Header().ID()).
		Timestamp(best.Header().Timestamp() + thor.BlockInterval()).
		TotalScore(best.Header().TotalScore() + 1).
		BaseFee(big.NewInt(thor.InitialBaseFee * 123)).
		TransactionFeatures(1).
		GasLimit(best.Header().GasLimit()).
		Build().
		WithSignature(sig[:])

	_, _, err = con.Process(chain.Repo().BestBlockSummary(), newBlock, newBlock.Header().Timestamp(), 0)
	assert.Contains(t, err.Error(), "block baseFee invalid: have 1230000000000000, want 10000000000000")
}

func TestConsensus_StopsEnergyAtHardfork(t *testing.T) {
	cfg := &thor.SoloFork
	cfg.HAYABUSA = 2
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	chain, err := testchain.NewWithFork(cfg, 2)
	assert.NoError(t, err)

	assert.NoError(t, chain.MintBlock())
	assert.NoError(t, chain.MintBlock())

	best := chain.Repo().BestBlockSummary()
	st := chain.Stater().NewState(best.Root())
	stop, err := builtin.Energy.Native(st, best.Header.Timestamp()).GetEnergyGrowthStopTime()
	assert.NoError(t, err)
	assert.Equal(t, best.Header.Timestamp(), stop)
}
