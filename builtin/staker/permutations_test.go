package staker

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
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

// PrintActionTree prints a tree visualization of the action hierarchy
func PrintActionTree(action Action) {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))

	log.Info("📋 Action Execution Plan:")
	printActionTreeHelper(action, "", true, 0, log)
	log.Info("🏁 End of execution plan\n")
}

func printActionTreeHelper(action Action, prefix string, isLast bool, parentBlock int, log *slog.Logger) {
	// Current node
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	// Calculate block advancement for this action
	blockAdvancement := 0
	if mpb := action.MinParentBlocksRequired(); mpb != nil {
		blockAdvancement = *mpb
	}

	// Calculate the block where this action will execute
	executionBlock := parentBlock + blockAdvancement

	// Format: ActionName @ block X (parentBlock + advancement)
	actionInfo := fmt.Sprintf("%s @ block %d (%d + %d)",
		action.Name(), executionBlock, parentBlock, blockAdvancement)

	log.Info(fmt.Sprintf("%s%s%s", prefix, connector, actionInfo))

	// Prepare prefix for children
	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	// Process next actions - they will execute after this action's execution block
	nextActions := action.Next()
	for i, nextAction := range nextActions {
		isLastChild := i == len(nextActions)-1
		printActionTreeHelper(nextAction, childPrefix, isLastChild, executionBlock, log)
	}
}

// RunWithTree prints the action tree and then executes it
func RunWithTree(s *testStaker, action Action, currentBlk int) error {
	PrintActionTree(action)
	return Runner(s, action, currentBlk)
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
		start := currentBlk
		end := currentBlk + *mpb
		interval := HousekeepingInterval
		first := ((start + interval - 1) / interval) * interval

		housekeepingCount := 0
		for cblk := first; cblk <= end; cblk += interval {
			log.Debug("🧹 Running housekeeping", "block", cblk)
			_, err := s.Housekeep(uint32(cblk))
			if err != nil {
				log.Error("❌ Housekeeping failed", "block", cblk, "error", err)
				return fmt.Errorf("housekeeping failed at block %d: %w", cblk, err)
			}
			housekeepingCount++
		}

		if housekeepingCount > 0 {
			log.Info("✅ Housekeeping completed", "operations_count", housekeepingCount)
		}

		currentBlk += *mpb
		log.Debug("⬆️ Block advancement completed", "new_block", currentBlk)
	}

	log.Info("⚡ Executing action", "action", action.Name(), "block", currentBlk)
	executionErr := action.Execute(s, currentBlk)
	checkErr := action.Check(s, currentBlk)
	switch {
	case executionErr != nil && checkErr != nil:
	case executionErr == nil && checkErr == nil:
		break
	default:
		return fmt.Errorf("%s Check Mismatch: ExecutionErr: %s - CheckErr %s", action.Name(), executionErr, checkErr)
	}

	log.Debug("✅ Action execution + check passed", "action", action.Name())

	nextActions := action.Next()
	for i, next := range nextActions {
		log.Debug("➡️ Starting next action", "sequence", i+1, "action", next.Name())
		if err := Runner(CloneStaker(s), next, currentBlk); err != nil {
			log.Error("❌ Next action failed", "sequence", i+1, "action", next.Name(), "error", err)
			return err
		}
	}

	log.Info("🎉 Action completed successfully", "action", action.Name())
	return nil
}

func TestPermutations(t *testing.T) {
	staker := newTestStaker()

	validatorID := thor.BytesToAddress([]byte("validator"))
	endorserID := thor.BytesToAddress([]byte("endorser"))

	epoch := HousekeepingInterval

	// Use original block timings (no modifications)
	originalModifier := func(original *int) *int {
		return original
	}

	signalExitMinBlock := originalModifier(&epoch)
	withDrawMinBlockCalc := convertNilBlock(signalExitMinBlock) + epoch + int(thor.CooldownPeriod())
	withDrawMinBlock := originalModifier(&withDrawMinBlockCalc)

	// Compose the flow explicitly: AddValidation -> SignalExit -> Withdraw
	action := NewValidationAction(
		originalModifier(nil),
		validatorID,
		endorserID,
		thor.LowStakingPeriod(),
		MinStakeVET,
		NewSignalExitAction(signalExitMinBlock, validatorID, endorserID,
			NewWithDrawAction(withDrawMinBlock, validatorID, endorserID),
		),
		NewWithDrawAction(withDrawMinBlock, validatorID, endorserID),
	)

	require.NoError(t, RunWithTree(staker, action, 0))
}

func TestRandomPermutations(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < 1_000; i++ {
		t.Run(fmt.Sprintf("iteration-%d", i+1), func(t *testing.T) {
			// Create per-action random modifier function
			randomModifier := func(original *int) *int {
				// Generate new random values for this iteration
				X := rand.Intn(360) + 1 // 1-360
				Y := rand.Intn(360) + 1 // 1-360

				// Randomly choose "above" or "below" for this action
				useAbove := rand.Intn(2) == 1

				var result *int
				if useAbove {
					// "above" permutation
					if original == nil {
						result = &Y
					} else {
						value := *original + Y
						result = &value
					}
				} else {
					// "below" permutation
					if original == nil {
						result = nil
					} else {
						value := *original - X
						if value <= 0 {
							result = nil
						} else {
							result = &value
						}
					}
				}

				return result
			}

			staker := newTestStaker()
			validatorID := thor.BytesToAddress([]byte("validator"))
			endorserID := thor.BytesToAddress([]byte("endorser"))

			epoch := HousekeepingInterval

			signalExitMinBlock := randomModifier(&epoch)
			withDrawMinBlockCalc := convertNilBlock(signalExitMinBlock) + epoch + int(thor.CooldownPeriod())
			withDrawMinBlock := randomModifier(&withDrawMinBlockCalc)

			// Compose the flow explicitly: AddValidation -> SignalExit -> Withdraw
			action := NewValidationAction(
				randomModifier(nil),
				validatorID,
				endorserID,
				thor.LowStakingPeriod(),
				MinStakeVET,
				NewSignalExitAction(signalExitMinBlock, validatorID, endorserID,
					NewWithDrawAction(withDrawMinBlock, validatorID, endorserID),
				),
				NewWithDrawAction(withDrawMinBlock, validatorID, endorserID),
			)

			require.NoError(t, RunWithTree(staker, action, 0))
		})
	}
}

// CloneStaker creates a deep copy of the testStaker with cloned state
func CloneStaker(ts *testStaker) *testStaker {
	// Get the current state root by creating and committing a stage
	stage, err := ts.state.Stage(trie.Version{})
	if err != nil {
		panic(err) // For testing, this should not fail
	}

	// Commit the stage to persist all trie data
	currentRoot, err := stage.Commit()
	if err != nil {
		panic(err) // For testing, this should not fail
	}

	// Create a new state from the same database but using the committed root
	// This approach shares the database but creates independent state trees
	clonedState := ts.state.Checkout(trie.Root{Hash: currentRoot})

	// Create a new staker with the cloned state
	clonedStaker := New(ts.addr, clonedState, params.New(ts.addr, clonedState), nil)

	return &testStaker{
		addr:   ts.addr,
		state:  clonedState,
		Staker: clonedStaker,
	}
}

func convertNilBlock(i *int) int {
	base := 0
	if i != nil {
		base = *i
	}
	return base
}
