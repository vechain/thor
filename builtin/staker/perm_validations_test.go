package staker

import (
	"fmt"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
	"math/big"
)

// NewAddValidationAction builds an AddValidation action with explicit parameters.
// Note: *Staker is NOT captured; it's passed at Execute-time as requested.
// Chaining is explicit: pass follow-up actions via `next ...Action`.
func NewAddValidationAction(
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
			func(ctx *ExecutionContext, s *testStaker, blk int) error {
				if err := s.AddValidation(validator, endorser, period, stake); err != nil {
					return fmt.Errorf("AddValidation failed: %w", err)
				}
				// Update context with initial stake and start block
				ctx.InitialStake = stake
				ctx.ValidationStartBlock = &blk
				ctx.LastActionAmount = stake // For display purposes
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, s *testStaker, blk int) error {
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
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				if err := staker.SignalExit(validationID, endorserID, uint32(blk)); err != nil {
					return err
				}
				// Update context with signal exit block
				ctx.SignalExitBlock = &blk
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
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

// NewWithdrawAction composes a Withdraw action.
func NewWithdrawAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	endorserID thor.Address,
) Action {
	var actualWithdrawnAmount uint64 // Store actual amount from execution

	return NewActionBuilder("Withdraw").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				amount, err := staker.WithdrawStake(validationID, endorserID, uint32(blk))
				actualWithdrawnAmount = amount
				ctx.LastActionAmount = amount
				return err
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				_, err := staker.GetValidation(validationID)
				if err != nil {
					return fmt.Errorf("GetValidation failed: %w", err)
				}

				expectedAmount := calculateExpectedValidationWithdrawAmount(ctx, blk)
				if actualWithdrawnAmount != expectedAmount {
					return fmt.Errorf("Amount validation failed: expected %d, got %d", expectedAmount, actualWithdrawnAmount)
				}

				return nil
			}).
		Build()
}

func NewIncreaseStakeAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	endorserID thor.Address,
	amount uint64,
	next ...Action,
) Action {
	return NewActionBuilder("IncreaseStake").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				if err := staker.IncreaseStake(validationID, endorserID, amount); err != nil {
					return err
				}
				// Add positive adjustment to context if context is provided
				ctx.StakeAdjustments = append(ctx.StakeAdjustments, StakeAdjustment{
					Block:  blk,
					Amount: int64(amount),
				})
				ctx.LastActionAmount = amount // For display purposes
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				val, err := staker.GetValidation(validationID)
				if err != nil {
					return fmt.Errorf("Check IncreaseStake failed, validator not found: %w", err)
				}
				if val.Endorser != endorserID {
					return fmt.Errorf("Check IncreaseStake failed, endorser not found")
				}
				if val.Status == validation.StatusExit {
					return fmt.Errorf("Check IncreaseStake failed, validator exited")
				}
				if val.Status == validation.StatusActive && val.ExitBlock != nil {
					return fmt.Errorf("Check IncreaseStake failed, validator has signaled exit")
				}
				if err := staker.validateStakeIncrease(validationID, val, amount); err != nil {
					return fmt.Errorf("Check IncreaseStake failed, validateStakeIncrease failed: %w", err)
				}
				return nil
			}).
		WithNext(next...).
		Build()
}

func NewDecreaseStakeAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	endorserID thor.Address,
	amount uint64,
	next ...Action,
) Action {
	return NewActionBuilder("DecreaseStake").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				if err := staker.DecreaseStake(validationID, endorserID, amount); err != nil {
					return err
				}
				// Add negative adjustment to context if context is provided
				ctx.StakeAdjustments = append(ctx.StakeAdjustments, StakeAdjustment{
					Block:  blk,
					Amount: -int64(amount),
				})
				ctx.LastActionAmount = amount // For display purposes (positive value)
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				val, err := staker.GetValidation(validationID)
				if err != nil {
					return fmt.Errorf("Check DecreaseStake failed, validator not found: %w", err)
				}
				if amount > MaxStakeVET-MinStakeVET {
					return fmt.Errorf("Check DecreaseStake failed, decrease amount is too large")
				}
				if val.Endorser != endorserID {
					return fmt.Errorf("Check DecreaseStake failed, endorser not found")
				}
				if val.Status == validation.StatusExit {
					return fmt.Errorf("Check DecreaseStake failed, validator exited")
				}
				if val.Status == validation.StatusActive && val.ExitBlock != nil {
					return fmt.Errorf("Check DecreaseStake failed, validator has signaled exit")
				}
				return nil
			}).
		WithNext(next...).
		Build()
}

func NewSetBeneficiaryAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	endorserID thor.Address,
	beneficiary thor.Address,
	next ...Action,
) Action {
	return NewActionBuilder("SetBeneficiary").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				return staker.SetBeneficiary(validationID, endorserID, beneficiary)
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				val, err := staker.GetValidation(validationID)
				if err != nil {
					return fmt.Errorf("Check SetBeneficiary failed, validator not found: %w", err)
				}
				if val.Endorser != endorserID {
					return fmt.Errorf("Check SetBeneficiary failed, endorser not found")
				}
				if val.Status == validation.StatusExit || val.ExitBlock != nil {
					return fmt.Errorf("Check SetBeneficiary failed, validator has exited or signaled exit")
				}
				return nil
			}).
		WithNext(next...).
		Build()
}

func NewAddDelegationAction(
	minParentBlocksRequired *int,
	validationID thor.Address,
	stake uint64,
	multiplier uint8,
	next ...Action,
) Action {
	return NewActionBuilder("AddDelegation").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				_, err := staker.AddDelegation(validationID, stake, multiplier, uint32(blk))
				if err != nil {
					return fmt.Errorf("AddDelegation failed : %w", err)
				}
				// Update context with delegation info if context is provided
				if ctx != nil {
					ctx.InitialDelegationStake = stake
					ctx.DelegationStartBlock = &blk
					ctx.LastActionAmount = stake // For display purposes
				}
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				if stake <= 0 {
					return NewReverts("stake must be greater than 0")
				}

				if multiplier == 0 {
					return NewReverts("multiplier cannot be 0")
				}
				// ensure validation is ok to receive a new delegation
				val, err := staker.Staker.getValidationOrRevert(validationID)
				if err != nil {
					return err
				}

				if val.Status != validation.StatusQueued && val.Status != validation.StatusActive {
					return NewReverts("validation is not queued or active")
				}

				// delegations cannot be added to a validator that has signaled to exit
				if val.ExitBlock != nil {
					return NewReverts("cannot add delegation to exiting validator")
				}

				// validate that new TVL is <= Max stake
				if err = staker.Staker.validateStakeIncrease(validationID, val, stake); err != nil {
					return err
				}
				return nil
			}).
		WithNext(next...).
		Build()
}

func NewSignalExitDelegationAction(
	minParentBlocksRequired *int,
	delegationID *big.Int,
	next ...Action,
) Action {
	return NewActionBuilder("SignalExitDelegation").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				if err := staker.SignalDelegationExit(delegationID, uint32(blk)); err != nil {
					return err
				}
				// Update context with delegation exit block if context is provided
				if ctx != nil {
					ctx.DelegationExitBlock = &blk
				}
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				del, err := staker.Staker.delegationService.GetDelegation(delegationID)
				if err != nil {
					return err
				}
				if del == nil {
					return NewReverts("delegation is empty")
				}
				if del.LastIteration == nil {
					return NewReverts("delegation is not signaled exit")
				}
				if del.Stake == 0 {
					return NewReverts("delegation has already been withdrawn")
				}

				// there can never be a delegation pointing to a non-existent validation
				// if the validation does not exist it's a system error
				val, err := staker.validationService.GetExistingValidation(del.Validation)
				if err != nil {
					return err
				}

				// ensure delegation can be signaled ( delegation has started and has not ended )
				started, err := del.Started(val, uint32(blk))
				if err != nil {
					return err
				}
				if !started {
					return NewReverts("delegation has not started yet, funds can be withdrawn")
				}
				ended, err := del.Ended(val, uint32(blk))
				if err != nil {
					return err
				}
				if ended {
					return NewReverts("delegation has ended, funds can be withdrawn")
				}

				return nil
			}).
		WithNext(next...).
		Build()
}

func NewWithdrawDelegationAction(
	minParentBlocksRequired *int,
	delegationID *big.Int,
	next ...Action,
) Action {
	var actualWithdrawnAmount uint64 // Store actual amount from execution

	return NewActionBuilder("WithdrawDelegation").
		WithMinParentBlocksRequired(minParentBlocksRequired).
		WithExecute(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				amount, err := staker.WithdrawDelegation(delegationID, uint32(blk))
				if err != nil {
					return fmt.Errorf("WithdrawDelegation failed : %w", err)
				}
				actualWithdrawnAmount = amount
				if ctx != nil {
					ctx.LastActionAmount = amount // For display purposes
				}
				return nil
			}).
		WithCheck(
			func(ctx *ExecutionContext, staker *testStaker, blk int) error {
				del, err := staker.delegationService.GetDelegation(delegationID)
				if err != nil {
					return err
				}

				if del == nil {
					return NewReverts("delegation is empty")
				}

				// there can never be a delegation pointing to a non-existent validation
				// if the validation does not exist it's a system error
				val, err := staker.validationService.GetExistingValidation(del.Validation)
				if err != nil {
					return err
				}

				// ensure the delegation is either queued or finished
				started, err := del.Started(val, uint32(blk))
				if err != nil {
					return err
				}
				finished, err := del.Ended(val, uint32(blk))
				if err != nil {
					return err
				}
				if started && !finished {
					return NewReverts("delegation is not eligible for withdraw")
				}

				expectedAmount := calculateExpectedDelegationWithdrawAmount(ctx, blk)
				if actualWithdrawnAmount != expectedAmount {
					return fmt.Errorf("Delegation amount validation failed: expected %d, got %d", expectedAmount, actualWithdrawnAmount)
				}

				return nil
			}).
		WithNext(next...).
		Build()
}
