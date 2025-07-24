package aggregation

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/renewal"
	"github.com/vechain/thor/v2/thor"
)

var slotAggregations = thor.BytesToBytes32([]byte("aggregated-delegations"))

type Service struct {
	storage *solidity.Mapping[thor.Address, *Aggregation]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		storage: solidity.NewMapping[thor.Address, *Aggregation](sctx, slotAggregations),
	}
}

func (s *Service) GetAggregation(validationID thor.Address) (*Aggregation, error) {
	d, err := s.storage.Get(validationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator aggregation")
	}

	// never return nil pointer aggregations
	if d == nil || d.LockedVET == nil {
		d = newAggregation()
	}
	return d, nil
}

func (s *Service) AddPendingVET(validationID thor.Address, weight *big.Int, stake *big.Int) error {
	agg, err := s.GetAggregation(validationID)
	if err != nil {
		return err
	}

	agg.PendingVET = big.NewInt(0).Add(agg.PendingVET, stake)
	agg.PendingWeight = big.NewInt(0).Add(agg.PendingWeight, weight)

	return s.storage.Set(validationID, agg, false)
}

func (s *Service) SubPendingVet(validationID thor.Address, stake *big.Int, weight *big.Int) error {
	agg, err := s.GetAggregation(validationID)
	if err != nil {
		return err
	}

	agg.PendingVET = big.NewInt(0).Sub(agg.PendingVET, stake)
	agg.PendingWeight = big.NewInt(0).Sub(agg.PendingWeight, weight)

	return s.storage.Set(validationID, agg, false)
}

func (s *Service) SubWithdrawableVET(validationID thor.Address, stake *big.Int) error {
	agg, err := s.GetAggregation(validationID)
	if err != nil {
		return err
	}

	if agg.WithdrawableVET.Cmp(stake) < 0 {
		return errors.New("not enough withdraw VET")
	}

	agg.WithdrawableVET = big.NewInt(0).Sub(agg.WithdrawableVET, stake)

	return s.storage.Set(validationID, agg, false)
}

func (s *Service) Renew(validationID thor.Address) (*renewal.Renewal, error) {
	agg, err := s.GetAggregation(validationID)
	if err != nil {
		return nil, err
	}
	renew := agg.Renew()

	if err = s.storage.Set(validationID, agg, false); err != nil {
		return nil, err
	}

	return renew, nil
}

func (s *Service) Exit(validationID thor.Address) (*renewal.Exit, error) {
	agg, err := s.GetAggregation(validationID)
	if err != nil {
		return nil, err
	}
	exit := agg.Exit()

	if err = s.storage.Set(validationID, agg, false); err != nil {
		return nil, err
	}

	return exit, nil
}

func (s *Service) SignalExit(id thor.Address) error {
	agg, err := s.GetAggregation(id)
	if err != nil {
		return err
	}

	agg.ExitingVET = big.NewInt(0).Add(agg.ExitingVET, agg.LockedVET)
	agg.ExitingWeight = big.NewInt(0).Add(agg.ExitingWeight, agg.LockedWeight)

	return s.storage.Set(id, agg, false)
}
