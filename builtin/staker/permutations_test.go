package staker

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"log/slog"
	"os"
	"testing"
)

// ----------------------
// Constants
// ----------------------

const (
	HousekeepingInterval = 180 // blocks between housekeeping operations
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
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))

	log.Debug("🚀 Starting action execution",
		"action", action.Name(),
		"current_block", currentBlk)

	// Use minParentBlocksRequired to drive housekeeping cadence / block skipping.
	if mpb := action.MinParentBlocksRequired(); mpb != nil {
		log.Debug("⏭️ Block advancement required",
			"blocks_to_advance", *mpb,
			"target_block", currentBlk+*mpb)

		// Ensure Housekeep runs every housekeepingInterval blocks, possibly multiple times if we've jumped
		housekeepingCount := 0
		for cblk := currentBlk; cblk <= currentBlk+*mpb; cblk++ {
			if cblk%HousekeepingInterval == 0 {
				log.Debug("🧹 Running housekeeping", "block", cblk)
				_, err := s.Housekeep(uint32(cblk))
				if err != nil {
					log.Error("❌ Housekeeping failed", "block", cblk, "error", err)
					return fmt.Errorf("housekeeping failed at block %d: %w", cblk, err)
				}
				housekeepingCount++
			}
		}

		if housekeepingCount > 0 {
			log.Info("✅ Housekeeping completed", "operations_count", housekeepingCount)
		}

		currentBlk += *mpb
		log.Debug("⬆️ Block advancement completed", "new_block", currentBlk)
	}

	log.Info("⚡ Executing action", "action", action.Name(), "block", currentBlk)
	err := action.Execute(s, currentBlk)
	if err != nil {
		log.Error("❌ Action execution failed", "action", action.Name(), "error", err)
		return fmt.Errorf("action %s execution failed: %w", action.Name(), err)
	}
	log.Debug("✅ Action executed successfully", "action", action.Name())

	log.Debug("🔍 Running action checks", "action", action.Name())
	checkErr := action.Check(s, currentBlk)
	if checkErr != nil {
		log.Error("❌ Action check failed", "action", action.Name(), "error", checkErr)
		return fmt.Errorf("action %s check failed: %w", action.Name(), checkErr)
	}
	log.Debug("✅ Action checks passed", "action", action.Name())

	nextActions := action.Next()
	if len(nextActions) > 0 {
		log.Info("🔄 Processing next actions", "count", len(nextActions))
		for i, next := range nextActions {
			log.Debug("➡️ Starting next action", "sequence", i+1, "action", next.Name())
			if err := Runner(s, next, currentBlk); err != nil {
				log.Error("❌ Next action failed", "sequence", i+1, "action", next.Name(), "error", err)
				return err
			}
		}
	}

	log.Info("🎉 Action completed successfully", "action", action.Name())
	return nil
}

func TestPermutation(t *testing.T) {
	staker := newTestStaker()

	validatorID := thor.BytesToAddress([]byte("validator"))
	endorserID := thor.BytesToAddress([]byte("endorser"))

	epoch := HousekeepingInterval

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
