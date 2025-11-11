// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/state"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type HayabusaTest struct {
	*testchain.Chain
	t       *testing.T
	mbp     int
	genesis *genesis.Genesis
}

type Config struct {
	HayabusaFork     uint32
	MBP              uint64
	EpochLength      uint32
	TransitionPeriod uint32
}

func NewHayabusaTest(t *testing.T, config Config) *HayabusaTest {
	thor.MockBlocklist([]string{})
	fc := &thor.ForkConfig{
		HAYABUSA: config.HayabusaFork,
	}
	gene := CreateGenesis(fc, config)
	chain, err := testchain.NewIntegrationTestChainWithGenesis(gene, fc, thor.EpochLength())
	require.NoError(t, err)

	t.Cleanup(func() {
		chain.Close()
	})

	return &HayabusaTest{
		t:       t,
		Chain:   chain,
		mbp:     int(config.MBP),
		genesis: gene,
	}
}

// MintBlock finds the correct packer (ie. the scheduled proposer) and mints a block with the given transactions.
func (h *HayabusaTest) MintBlock(txs ...*tx.Transaction) bool {
	var (
		flow  *packer.Flow
		when  uint64 = math.MaxUint64
		isPos bool
		acc   genesis.DevAccount
	)

	for i := range h.mbp {
		p := packer.New(h.Repo(), h.Stater(), genesis.DevAccounts()[i].Address, nil, h.GetForkConfig(), 0)

		currentFlow, isDPos, err := p.Schedule(h.Repo().BestBlockSummary(), uint64(time.Now().Unix()))
		require.NoError(h.t, err)

		if currentFlow.When() < when {
			when = currentFlow.When()
			flow = currentFlow
			isPos = isDPos
			acc = genesis.DevAccounts()[i]
		}
	}
	require.NotNil(h.t, flow)

	for _, trx := range txs {
		require.NoError(h.t, flow.Adopt(trx))
	}

	block, stage, receipts, err := flow.Pack(acc.PrivateKey, 0, false)
	require.NoError(h.t, err)
	require.NoError(h.t, h.AddBlock(block, stage, receipts))

	return isPos
}

// MakeTx creates and signs a transaction with given clauses and signer.
func (h *HayabusaTest) MakeTx(clauses []*tx.Clause, signer genesis.DevAccount) *tx.Transaction {
	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(h.Repo().ChainTag()).
		Expiration(1000).
		Clauses(clauses).
		Nonce(datagen.RandUint64()).
		Gas(1000000).
		Build()
	return tx.MustSign(trx, signer.PrivateKey)
}

// AddValidators adds validators to the staker contract
func (h *HayabusaTest) AddValidators() {
	method, ok := builtin.Staker.ABI.MethodByName("addValidation")
	require.True(h.t, ok)

	txs := make([]*tx.Transaction, h.mbp)

	for i := range h.mbp {
		callData, err := method.EncodeInput(genesis.DevAccounts()[i].Address, thor.LowStakingPeriod())
		require.NoError(h.t, err)
		clause := tx.NewClause(&builtin.Staker.Address).WithData(callData).WithValue(staker.MinStake)
		trx := h.MakeTx([]*tx.Clause{clause}, genesis.DevAccounts()[i])
		txs[i] = trx
	}

	h.MintBlock(txs...)
}

func (h *HayabusaTest) state() *state.State {
	best := h.Repo().BestBlockSummary()
	return h.Stater().NewState(best.Root())
}

// StakerDeployed checks whether the staker contract code is deployed
func (h *HayabusaTest) StakerDeployed() bool {
	code, err := h.state().GetCode(builtin.Staker.Address)
	require.NoError(h.t, err)
	return len(code) > 50
}

// StakerActive checks whether the staker contract indicates PoS is active
func (h *HayabusaTest) StakerActive() bool {
	staker := builtin.Staker.Native(h.state())
	active, err := staker.IsPoSActive()
	require.NoError(h.t, err)
	return active
}

// CreateGenesis create a test genesis for packer tests
func CreateGenesis(fc *thor.ForkConfig, c Config) *genesis.Genesis {
	largeNo := (*genesis.HexOrDecimal256)(new(big.Int).SetBytes(hexutil.MustDecode("0xffffffffffffffffffffffffffffffffff")))
	// Create accounts for all DevAccounts
	accounts := make([]genesis.Account, len(genesis.DevAccounts()))
	for i, devAcc := range genesis.DevAccounts() {
		accounts[i] = genesis.Account{
			Address: devAcc.Address,
			Balance: largeNo,
			Energy:  largeNo,
		}
	}

	authorities := make([]genesis.Authority, c.MBP)
	for i := range int(c.MBP) {
		authorities[i] = genesis.Authority{
			MasterAddress:   genesis.DevAccounts()[i].Address,
			EndorsorAddress: genesis.DevAccounts()[i].Address,
			Identity:        thor.BytesToBytes32([]byte("Block Signer")),
		}
	}

	gen := &genesis.CustomGenesis{
		LaunchTime: uint64(time.Now().Unix()),
		GasLimit:   thor.InitialGasLimit,
		ExtraData:  "packer test",
		Accounts:   accounts,
		Authority:  authorities,
		Params: genesis.Params{
			ExecutorAddress:   &genesis.DevAccounts()[0].Address,
			MaxBlockProposers: &c.MBP,
		},
		ForkConfig: fc,
		Config: &thor.Config{
			BlockInterval:       10,
			LowStakingPeriod:    12,
			MediumStakingPeriod: 30,
			HighStakingPeriod:   90,
			CooldownPeriod:      12,
			EpochLength:         c.EpochLength,
			HayabusaTP:          &c.TransitionPeriod,
		},
	}

	customNet, err := genesis.NewCustomNet(gen)
	if err != nil {
		panic(err)
	}
	return customNet
}

func TestFlow_Schedule_POS(t *testing.T) {
	chain := NewHayabusaTest(t, Config{
		HayabusaFork:     2,
		MBP:              5,
		EpochLength:      2,
		TransitionPeriod: 2,
	})

	// block 1: still poa
	assert.False(t, chain.MintBlock())

	// block 2: fork happens
	assert.False(t, chain.MintBlock())
	assert.True(t, chain.StakerDeployed())

	// block 3: add validators
	chain.AddValidators()

	// block 4: transition to poa
	assert.True(t, chain.MintBlock())
	assert.True(t, chain.StakerActive())
}

func TestPacker_StopsEnergyAtHardfork(t *testing.T) {
	cases := []struct {
		name       string
		hayabusa   uint32
		expectStop bool
	}{
		{"stops at hardfork block", 2, true},
		{"does not stop without fork", math.MaxUint32, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chain := NewHayabusaTest(t, Config{
				HayabusaFork:     tc.hayabusa,
				MBP:              3,
				EpochLength:      2,
				TransitionPeriod: 2,
			})

			chain.MintBlock()
			chain.MintBlock()

			best := chain.Repo().BestBlockSummary()
			stop, err := builtin.Energy.Native(chain.state(), best.Header.Timestamp()).GetEnergyGrowthStopTime()
			assert.NoError(t, err)
			if tc.expectStop {
				assert.Equal(t, best.Header.Timestamp(), stop)
			} else {
				assert.Equal(t, uint64(math.MaxUint64), stop)
			}
		})
	}
}

func TestFlow_Revert(t *testing.T) {
	chain := NewHayabusaTest(t, Config{
		HayabusaFork:     2,
		MBP:              3,
		EpochLength:      2,
		TransitionPeriod: 1,
	})

	// mint block 1: using PoA
	assert.False(t, chain.MintBlock())
	assert.False(t, chain.StakerDeployed())

	// mint block 2: deploy staker contract, still using PoA
	assert.False(t, chain.MintBlock())
	assert.True(t, chain.StakerDeployed())

	// mint block 3: add validator tx
	chain.AddValidators()

	// mint block 4: should switch to PoS
	assert.True(t, chain.MintBlock())

	oldStakerBalance, err := chain.state().GetBalance(builtin.Staker.Address)
	assert.NoError(t, err)
	oldBalance, err := chain.state().GetBalance(genesis.DevAccounts()[1].Address)
	assert.NoError(t, err)

	bestBlock, _ := chain.BestBlock()
	amount, _ := big.NewInt(0).SetString("1000000000000000000", 10)
	failingTransaction := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(chain.Repo().ChainTag()).
		Expiration(10).
		Nonce(1).
		Gas(3000000).
		MaxFeePerGas(bestBlock.Header().BaseFee()).
		MaxPriorityFeePerGas(big.NewInt(3000000)).
		Clause(tx.NewClause(&builtin.Staker.Address).WithData([]byte{
			0xc3, 0xc4, 0xb1, 0x38, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0xf0, 0x77, 0xb4, 0x91,
			0xb3, 0x55, 0xe6, 0x40, 0x48, 0xce, 0x21, 0xe3,
			0xa6, 0xfc, 0x47, 0x51, 0xee, 0xea, 0x77, 0xfa,

			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0e, 0x10,
		}).WithValue(amount)).
		Clause(tx.NewClause(&genesis.DevAccounts()[1].Address).WithValue(amount)).
		Build()
	failingTransaction = tx.MustSign(failingTransaction, genesis.DevAccounts()[0].PrivateKey)

	// mint block 5: full PoS
	assert.True(t, chain.MintBlock(failingTransaction))

	stakerBalance, err := chain.state().GetBalance(builtin.Staker.Address)
	assert.NoError(t, err)
	assert.Equal(t, oldStakerBalance, stakerBalance)

	balance, err := chain.state().GetBalance(genesis.DevAccounts()[1].Address)
	assert.NoError(t, err)
	assert.Equal(t, oldBalance, balance)
}
