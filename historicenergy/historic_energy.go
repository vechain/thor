package historic_energy

import (
	"math/big"
	"time"

	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
)

func GetHistoricGenerationRates(chain *chain.Chain, stater *state.Stater, account state.Account, authorityContract *authority.Authority) (*big.Int, error) {
	// Get Account last state change from chain
	header, err := chain.FindBlockHeaderByTimestamp(account.BlockTime, 1)

	if err != nil {
		return nil, err
	}
	lastAccountChangeBlock := header.Number()

	// Get Best Block from chain
	bestBlockHeader, err := chain.FindBlockHeaderByTimestamp(uint64(time.Now().Unix()), 1)

	if err != nil {
		return nil, err
	}

	bestBlock := bestBlockHeader.Number()

	sum := account.Energy

	// Aggregate generation rates
	for i := lastAccountChangeBlock; i <= bestBlock; i++ {

		b, err := chain.GetBlock(i)

		if err != nil {
			return nil, err
		}

		// Need to get state of blocks one by one
		state := stater.NewState(b.Header().StateRoot(), i, 0, 0)

		validatorGenRate, _, err := authorityContract.CalcGenerationRates(state)

		if err != nil {
			return nil, err
		}

		sum = new(big.Int).Add(sum, validatorGenRate)

	}

	return sum, nil
}
