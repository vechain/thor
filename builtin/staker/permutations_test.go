package staker

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"testing"
)

// ----------------------
// Interfaces & Typedefs
// ----------------------

// Action is now produced by the generic builder. It keeps *Staker runtime-bound.
type Action interface {
	Name() string
	Execute(s *testStaker, blk int) error
	Check(s *testStaker, blk int) error
	Next() []Action
	// Option A (typed): used by Runner to decide block skipping/housekeeping cadence.
	MinParentBlocksRequired() *int
}

// builtAction is the generic concrete type for all actions.
type builtAction struct {
	name                    string
	execute                 func(*testStaker, int) error
	check                   func(*testStaker, int) error
	next                    []Action
	minParentBlocksRequired *int
}

func (a *builtAction) Name() string { return a.name }

func (a *builtAction) Execute(s *testStaker, blk int) error {
	if a.execute == nil {
		return nil
	}
	return a.execute(s, blk)
}

func (a *builtAction) Check(s *testStaker, blk int) error {
	if a.check == nil {
		return nil
	}
	return a.check(s, blk)
}

func (a *builtAction) Next() []Action { return a.next }

func (a *builtAction) MinParentBlocksRequired() *int {
	return a.minParentBlocksRequired
}

// ----------------------
// Action Builder
// ----------------------

type ActionBuilder struct {
	name                    string
	execute                 func(*testStaker, int) error
	check                   func(*testStaker, int) error
	next                    []Action
	minParentBlocksRequired *int
}

func NewActionBuilder(name string) *ActionBuilder {
	return &ActionBuilder{name: name}
}

func (b *ActionBuilder) WithExecute(fn func(*testStaker, int) error) *ActionBuilder {
	b.execute = fn
	return b
}

func (b *ActionBuilder) WithCheck(fn func(*testStaker, int) error) *ActionBuilder {
	b.check = fn
	return b
}

func (b *ActionBuilder) WithNext(next ...Action) *ActionBuilder {
	b.next = append(b.next, next...)
	return b
}

func (b *ActionBuilder) WithMinParentBlocksRequired(n *int) *ActionBuilder {
	b.minParentBlocksRequired = n
	return b
}

func (b *ActionBuilder) Build() Action {
	return &builtAction{
		name:                    b.name,
		execute:                 b.execute,
		check:                   b.check,
		next:                    b.next,
		minParentBlocksRequired: b.minParentBlocksRequired,
	}
}

// Runner executes an action, runs its checks, then recurses on Next().
// It now *exposes* and *uses* MinParentBlocksRequired() to control housekeeping cadence.
func Runner(s *testStaker, action Action, currentBlk int) error {
	fmt.Printf("▶ Executing: %s\n", action.Name())

	// Use minParentBlocksRequired to drive housekeeping cadence / block skipping.
	if mpb := action.MinParentBlocksRequired(); mpb != nil {
		// Ensure Housekeep runs every 180 blocks, possibly multiple times if we've jumped
		for cblk := currentBlk; cblk <= currentBlk+*mpb; cblk++ {
			if cblk%180 == 0 {
				_, err := s.Housekeep(uint32(cblk))
				if err != nil {
					return err
				}
			}
		}

		currentBlk += *mpb
	}

	err := action.Execute(s, currentBlk)
	checkErr := action.Check(s, currentBlk)
	switch {
	case checkErr != nil && err != nil:
	case checkErr == nil && err == nil:
		break
	default:
		return fmt.Errorf("Check failed: action %s failed: ActionErr=%s CheckErr=%s", action.Name(), err, checkErr)
	}

	for _, next := range action.Next() {
		if err := Runner(s, next, currentBlk); err != nil {
			return err
		}
	}
	return nil
}

func TestPermutation(t *testing.T) {
	staker := newTestStaker()

	validatorID := thor.BytesToAddress([]byte("validator"))
	endorserID := thor.BytesToAddress([]byte("endorser"))

	epoch := 180

	// Compose the flow explicitly: AddValidation -> SignalExit
	addValidation := NewValidationAction(
		nil,
		validatorID,
		endorserID,
		thor.LowStakingPeriod(),
		MinStakeVET,
		NewSignalExitAction(&epoch, validatorID, endorserID),
	)

	require.NoError(t, Runner(staker, addValidation, 0))
}

func NewStaker() *Staker {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("staker"))
	return New(addr, st, params.New(addr, st), nil)
}
