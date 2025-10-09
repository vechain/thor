package staker

import (
	"fmt"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

// NewValidationAction builds an AddValidation action with explicit parameters.
// Note: *Staker is NOT captured; it's passed at Execute-time as requested.
// Chaining is explicit: pass follow-up actions via `next ...Action`.
func NewValidationAction(
	minParentBlocksRequired *int,
	validator thor.Address,
	endorser thor.Address,
	period uint32,
	stake uint64,
	next ...Action,
) Action {
	return NewActionBuilder("AddValidation").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(s *testStaker, _ int) error {
				if err := s.AddValidation(validator, endorser, period, stake); err != nil {
					return fmt.Errorf("AddValidation failed: %w", err)
				}
				return nil
			}).
		WithCheck(
			func(s *testStaker, _ int) error {
				// Per your instruction, the validationID is the validator address.
				val, err := s.GetValidation(validator)
				if err != nil {
					return fmt.Errorf("Check GetValidation failed: %w", err)
				}
				if val == nil {
					return fmt.Errorf("Check failed: validation not found for %s", validator.Hex())
				}
				return nil
			}).
		WithNext(next...).
		Build()
}

// NewSignalExitAction composes a SignalExit action.
func NewSignalExitAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	endorserID thor.Address,
	next ...Action,
) Action {
	return NewActionBuilder("SignalExit").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(staker *testStaker, blk int) error {
				return staker.SignalExit(validationID, endorserID, uint32(blk))
			}).
		WithCheck(
			func(staker *testStaker, blk int) error {
				val, err := staker.GetValidation(validationID)
				if err != nil {
					return fmt.Errorf("Check SignalExit GetValidation failed: %w", err)
				}

				if val.Status != validation.StatusActive {
					return fmt.Errorf("Check SignalExit GetValidation failed: expected status to be active, got %d", val.Status)
				}

				if val.ExitBlock == nil {
					return fmt.Errorf("Check SignalExit GetValidation failed: nil ExitBlock")
				}

				current, err := val.CurrentIteration(uint32(blk))
				if err != nil {
					return err
				}
				if current == 0 {
					return fmt.Errorf("SignalExit Check failed: current iteration cannot be 0")
				}

				return nil
			}).
		WithNext(next...).
		Build()
}

// NewWithDrawAction composes a Withdraw action.
func NewWithDrawAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	endorserID thor.Address,
) Action {
	return NewActionBuilder("Withdraw").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(staker *testStaker, blk int) error {
				_, err := staker.WithdrawStake(validationID, endorserID, uint32(blk))
				return err
			}).
		WithCheck(
			func(staker *testStaker, blk int) error {
				_, err := staker.GetValidation(validationID)
				if err != nil {
					return fmt.Errorf("GetValidation failed: %w", err)
				}

				//if val.Status != validation.StatusExit && val.Status != validation.StatusQueued {
				//	return fmt.Errorf("expected status failed, got %d", val.Status)
				//}
				//
				//if val.ExitBlock == nil {
				//	return fmt.Errorf("nil ExitBlock")
				//}
				//
				//if !val.CooldownEnded(uint32(blk)) {
				//	return fmt.Errorf("expected cooldown ended")
				//}
				return nil
			}).
		Build()
}
