// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"fmt"
	"log/slog"
	"maps"
	"math/big"
	"math/rand"
	"os"
	"runtime"
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

// ExecutionContext tracks the execution history for amount validation
type ExecutionContext struct {
	// Validation context
	InitialStake         uint64
	ValidationStartBlock *int
	SignalExitBlock      *int
	StakeAdjustments     []StakeAdjustment

	// Delegation context
	InitialDelegationStake uint64
	DelegationStartBlock   *int
	DelegationExitBlock    *int

	// For display purposes - last action amount
	LastActionAmount uint64

	// Current operation sequence (for Check function)
	CurrentSequence int
}

type StakeAdjustment struct {
	Block    int
	Amount   int64 // positive for increase, negative for decrease
	Sequence int   // execution sequence within the block
}

// Clone creates a deep copy of the ExecutionContext
func (ctx *ExecutionContext) Clone() *ExecutionContext {
	if ctx == nil {
		return &ExecutionContext{}
	}

	clone := &ExecutionContext{
		InitialStake:           ctx.InitialStake,
		InitialDelegationStake: ctx.InitialDelegationStake,
	}

	if ctx.ValidationStartBlock != nil {
		startBlock := *ctx.ValidationStartBlock
		clone.ValidationStartBlock = &startBlock
	}

	if ctx.SignalExitBlock != nil {
		exitBlock := *ctx.SignalExitBlock
		clone.SignalExitBlock = &exitBlock
	}

	if ctx.DelegationStartBlock != nil {
		startBlock := *ctx.DelegationStartBlock
		clone.DelegationStartBlock = &startBlock
	}

	if ctx.DelegationExitBlock != nil {
		exitBlock := *ctx.DelegationExitBlock
		clone.DelegationExitBlock = &exitBlock
	}

	clone.StakeAdjustments = make([]StakeAdjustment, len(ctx.StakeAdjustments))
	copy(clone.StakeAdjustments, ctx.StakeAdjustments)

	clone.LastActionAmount = ctx.LastActionAmount
	clone.CurrentSequence = ctx.CurrentSequence

	return clone
}

// StakeAdjustmentsNegativeSum returns the sum of all negative (decrease) stake adjustments
func (ctx *ExecutionContext) StakeAdjustmentsNegativeSum() uint64 {
	var sum uint64
	for _, adj := range ctx.StakeAdjustments {
		if adj.Amount < 0 {
			sum += uint64(-adj.Amount)
		}
	}
	return sum
}

// Action is now produced by the generic builder. It keeps *Staker runtime-bound.
type Action interface {
	Name() string
	Execute(ctx *ExecutionContext, s *testStaker, blk int) error
	Check(ctx *ExecutionContext, s *testStaker, blk int) error
	Next() []Action
	MinParentBlocksRequired() *int
}

// builtAction is the generic concrete type for all actions.
type builtAction struct {
	name                    string
	execute                 func(*ExecutionContext, *testStaker, int) error
	check                   func(*ExecutionContext, *testStaker, int) error
	next                    []Action
	minParentBlocksRequired *int
}

func (a *builtAction) Name() string { return a.name }

func (a *builtAction) Execute(ctx *ExecutionContext, s *testStaker, blk int) error {
	if a.execute == nil {
		return nil
	}
	return a.execute(ctx, s, blk)
}

func (a *builtAction) Check(ctx *ExecutionContext, s *testStaker, blk int) error {
	if a.check == nil {
		return nil
	}
	return a.check(ctx, s, blk)
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
	execute                 func(*ExecutionContext, *testStaker, int) error
	check                   func(*ExecutionContext, *testStaker, int) error
	next                    []Action
	minParentBlocksRequired *int
}

func NewActionBuilder(name string) *ActionBuilder {
	return &ActionBuilder{name: name}
}

func (b *ActionBuilder) WithExecute(fn func(*ExecutionContext, *testStaker, int) error) *ActionBuilder {
	b.execute = fn
	return b
}

func (b *ActionBuilder) WithCheck(fn func(*ExecutionContext, *testStaker, int) error) *ActionBuilder {
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

// ActionResult captures the execution and check results for an action
type ActionResult struct {
	ActionName   string
	ExecutionErr error
	CheckErr     error
	Block        int
	Amount       uint64 // Amount involved in the action (for display purposes)
}

// createLogger creates a structured logger with consistent configuration
func createLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

func printActionTreeWithResultsHelper(
	action Action,
	prefix string,
	isLast bool,
	parentBlock int,
	results map[string]*ActionResult,
	log *slog.Logger,
	usedResults map[string]bool,
) {
	// Current node
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	if prefix == "" {
		connector = "──> "
	}

	// Calculate block advancement for this action
	blockAdvancement := 0
	if mpb := action.MinParentBlocksRequired(); mpb != nil {
		blockAdvancement = *mpb
	}

	// Calculate the block where this action will execute
	executionBlock := parentBlock + blockAdvancement

	// Look up results for this action - since we don't know the sequence number during tree printing,
	// we need to find the matching key with the correct action and block that hasn't been used yet
	var result *ActionResult
	var found bool
	var matchingKey string
	for key, res := range results {
		if res.ActionName == action.Name() && res.Block == executionBlock && !usedResults[key] {
			result = res
			found = true
			matchingKey = key
			break
		}
	}
	if found {
		usedResults[matchingKey] = true
	}

	// Format result indicators
	execResult, checkResult := formatResultIndicators(found, result)

	// Format: ActionName @ block X (parentBlock + advancement) [ICON AMOUNT] ExecutionResult CheckResult
	amountDisplay := ""
	if found && result.Amount > 0 {
		icon := getAmountIcon(action.Name())
		if icon != "" {
			amountDisplay = fmt.Sprintf(" [%s %s]", icon, formatAmount(result.Amount))
		}
	}

	var actionInfo string
	stakingPeriod := int(thor.LowStakingPeriod())
	if blockAdvancement > stakingPeriod {
		excessBlocks := blockAdvancement - stakingPeriod
		actionInfo = fmt.Sprintf("%s @ block %d (%d + ⏳ StakingPeriod(%d)+%d)%s %s %s",
			action.Name(), executionBlock, parentBlock, stakingPeriod, excessBlocks, amountDisplay, execResult, checkResult)
	} else {
		actionInfo = fmt.Sprintf("%s @ block %d (%d + %d)%s %s %s",
			action.Name(), executionBlock, parentBlock, blockAdvancement, amountDisplay, execResult, checkResult)
	}

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
		printActionTreeWithResultsHelper(nextAction, childPrefix, isLastChild, executionBlock, results, log, usedResults)
	}
}

// RunWithResultTree executes actions and prints a tree with results
func RunWithResultTree(s *testStaker, action Action, currentBlk int) error {
	log := createLogger()
	results := make(map[string]*ActionResult)
	blockSequence := make(map[int]int) // Track sequence per block
	ctx := &ExecutionContext{}
	err := RunnerWithResults(s, action, currentBlk, results, ctx, blockSequence)
	usedResults := make(map[string]bool)
	printActionTreeWithResultsHelper(action, "", true, 0, results, log, usedResults)
	return err
}

// RunnerWithResults executes an action, runs its checks, captures results, then recurses on Next().
func RunnerWithResults(s *testStaker, action Action, currentBlk int, results map[string]*ActionResult, ctx *ExecutionContext, blockSequence map[int]int) error {
	log := createLogger()

	log.Debug("🚀 Starting action execution",
		"action", action.Name(),
		"current_block", currentBlk)

	// Advance blocks and run housekeeping if needed
	if mpb := action.MinParentBlocksRequired(); mpb != nil {
		log.Debug("⏭️ Block advancement required",
			"blocks_to_advance", *mpb,
			"target_block", currentBlk+*mpb)

		if err := runHousekeepingForBlockRange(s, currentBlk, currentBlk+*mpb, log); err != nil {
			return err
		}

		currentBlk += *mpb
		log.Debug("⬆️ Block advancement completed", "new_block", currentBlk)
	}

	log.Debug("⚡️ Executing action", "action", action.Name(), "block", currentBlk)

	// Create a unique key for this action execution with per-block sequence
	blockSequence[currentBlk]++
	sequenceNum := blockSequence[currentBlk]

	// Execute and check with context
	executionErr := action.Execute(ctx, s, currentBlk)

	// Update the sequence number on the last added StakeAdjustment (if any)
	if len(ctx.StakeAdjustments) > 0 {
		lastAdjustment := &ctx.StakeAdjustments[len(ctx.StakeAdjustments)-1]
		if lastAdjustment.Block == currentBlk && lastAdjustment.Sequence == 0 {
			lastAdjustment.Sequence = sequenceNum
		}
	}

	// Store current sequence in context for Check function to use
	ctx.CurrentSequence = sequenceNum
	checkErr := action.Check(ctx, s, currentBlk)
	actionKey := fmt.Sprintf("%s@%d#%d", action.Name(), currentBlk, sequenceNum)
	results[actionKey] = &ActionResult{
		ActionName:   action.Name(),
		ExecutionErr: executionErr,
		CheckErr:     checkErr,
		Block:        currentBlk,
		Amount:       ctx.LastActionAmount,
	}

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
		// Clone context for each branch to prevent interference
		// Each fork gets its own copy of blockSequence to handle parallel branches
		clonedBlockSequence := make(map[int]int)
		maps.Copy(clonedBlockSequence, blockSequence)
		clonedCtx := ctx.Clone()
		if err := RunnerWithResults(cloneStaker(s), next, currentBlk, results, clonedCtx, clonedBlockSequence); err != nil {
			log.Error("❌ Next action failed", "sequence", i+1, "action", next.Name(), "error", err)
			return err
		}
	}

	log.Debug("🎉 Action completed successfully", "action", action.Name())
	return nil
}

// runHousekeepingForBlockRange runs housekeeping for all intervals in a block range
func runHousekeepingForBlockRange(s *testStaker, start, end int, log *slog.Logger) error {
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
		log.Debug("🏡 Housekeeping completed", "operations_count", housekeepingCount)
	}
	return nil
}

// cloneStaker creates a deep copy of the testStaker with cloned state
func cloneStaker(ts *testStaker) *testStaker {
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
	if i != nil {
		return *i
	}
	return 0
}

// createRandomModifier creates a random modifier function for block advancements
func createRandomModifier() func(*int) *int {
	return func(original *int) *int {
		X := rand.Intn(360) + 1       //nolint:gosec // 1-360
		Y := rand.Intn(360) + 1       //nolint:gosec // 1-360
		useAbove := rand.Intn(2) == 1 //nolint:gosec

		if useAbove {
			// "above" permutation
			if original == nil {
				return &Y
			}
			value := *original + Y
			return &value
		}

		// "below" permutation
		if original == nil {
			return nil
		}
		value := *original - X
		if value <= 0 {
			return nil
		}
		return &value
	}
}

// formatResultIndicators formats execution and check result indicators
func formatResultIndicators(found bool, result *ActionResult) (string, string) {
	execResult := "❓"
	checkResult := "❓"

	if found {
		if result.ExecutionErr == nil {
			execResult = "✅"
		} else {
			execResult = "❌"
		}

		if result.CheckErr == nil {
			checkResult = "✅"
		} else {
			checkResult = "❌"
		}
	}

	return execResult, checkResult
}

// formatAmount formats an amount in millions (e.g., 25000000 -> "25M")
func formatAmount(amount uint64) string {
	if amount == 0 {
		return ""
	}
	millions := amount / 1000000
	if amount%1000000 == 0 {
		return fmt.Sprintf("%dM", millions)
	}
	return fmt.Sprintf("%.1fM", float64(amount)/1000000)
}

// getAmountIcon returns the appropriate icon for an action type
func getAmountIcon(actionName string) string {
	switch actionName {
	case "AddValidation", "IncreaseStake", "AddDelegation":
		return "⬆️"
	case "DecreaseStake":
		return "⬇️"
	case "Withdraw", "WithdrawDelegation":
		return "💵"
	default:
		return ""
	}
}

// --- Permutation Manager Skeleton ---

type permutationManager struct {
	actionConstructors map[string]func(next ...Action) Action
}

// Generate all unique permutations of the provided set of actions with their counts.
// This generates ONLY complete permutations that include ALL actions from the config.
func (pm *permutationManager) GenerateAllPermutations(config map[string]int) [][]string {
	const headKey = "NewValidationAction"
	countHead := config[headKey]
	if countHead < 1 {
		return nil // no permutations allowed if no NewValidationAction
	}

	// Build complete action list excluding ONE head action to be inserted at front.
	var baseList []string
	for name, count := range config {
		c := count
		if name == headKey {
			c-- // We've reserved one for the head
		}
		for range c {
			baseList = append(baseList, name)
		}
	}

	if len(baseList) == 0 {
		// Just [headKey] is the only permutation
		return [][]string{{headKey}}
	}

	var results [][]string
	var permute func([]string, int)
	permute = func(arr []string, l int) {
		// Only generate complete permutations (when we've reached the end)
		if l == len(arr) {
			p := make([]string, len(arr))
			copy(p, arr)
			// Always prepend headKey to create complete scenario
			results = append(results, append([]string{headKey}, p...))
			return
		}

		// Use deduplication to avoid generating identical permutations
		dedup := make(map[string]bool)
		for i := l; i < len(arr); i++ {
			if dedup[arr[i]] {
				continue
			}
			dedup[arr[i]] = true
			arr[l], arr[i] = arr[i], arr[l]
			permute(arr, l+1)
			arr[l], arr[i] = arr[i], arr[l]
		}
	}

	permute(baseList, 0)
	return results
}

// InstantiateChain creates a chain of Action objects from a permutation order
func (pm *permutationManager) InstantiateChain(order []string) Action {
	if len(order) == 0 {
		return nil
	}

	var makeAction func(idx int) Action
	makeAction = func(idx int) Action {
		if idx >= len(order) {
			return nil
		}
		tail := makeAction(idx + 1)
		ctor := pm.actionConstructors[order[idx]]
		if ctor == nil {
			panic("unknown action: " + order[idx])
		}
		if tail != nil {
			return ctor(tail)
		}
		return ctor()
	}
	return makeAction(0)
}

// Permutation tests
func TestPermutation(t *testing.T) {
	staker := newTestStaker()

	validatorID := thor.BytesToAddress([]byte("validator"))
	endorserID := thor.BytesToAddress([]byte("endorser"))
	delegationID := big.NewInt(1)

	// Block advancement values for all green execution
	epoch := HousekeepingInterval
	stakingPeriod := int(thor.LowStakingPeriod())

	addValidationBlocks := 0                  // AddValidation at block 0
	addDelegationBlocks := 1                  // AddDelegation at block 1
	increaseStakeBlocks1 := epoch             // First IncreaseStake after epoch
	increaseStakeBlocks2 := stakingPeriod     // Second IncreaseStake after staking period
	decreaseStakeBlocks1 := epoch             // First DecreaseStake after epoch
	decreaseStakeBlocks2 := stakingPeriod     // Second DecreaseStake after staking period
	signalExitDelegationBlocks := epoch       // SignalExitDelegation after epoch
	withdrawDelegationBlocks := stakingPeriod // WithdrawDelegation after staking period
	signalExitBlocks := 1                     // SignalExit after 1 block
	withdrawBlocks := stakingPeriod           // Withdraw after staking period

	action := NewAddValidationAction(
		&addValidationBlocks,
		validatorID,
		endorserID,
		thor.LowStakingPeriod(),
		MinStakeVET,
		NewAddDelegationAction(&addDelegationBlocks, validatorID, MinStakeVET, uint8(1),
			NewIncreaseStakeAction(&increaseStakeBlocks1, validatorID, endorserID, MinStakeVET,
				NewIncreaseStakeAction(&increaseStakeBlocks2, validatorID, endorserID, MinStakeVET,
					NewDecreaseStakeAction(&decreaseStakeBlocks1, validatorID, endorserID, MinStakeVET,
						NewDecreaseStakeAction(&decreaseStakeBlocks2, validatorID, endorserID, MinStakeVET,
							NewSignalExitDelegationAction(&signalExitDelegationBlocks, delegationID,
								NewWithdrawDelegationAction(&withdrawDelegationBlocks, delegationID,
									NewSignalExitAction(&signalExitBlocks, validatorID, endorserID,
										NewWithdrawAction(&withdrawBlocks, validatorID, endorserID),
									),
								),
							),
						),
					),
				),
			),
		),
	)

	require.NoError(t, RunWithResultTree(staker, action, 0))
}

func TestRandomPermutationManager(t *testing.T) {
	// If RUN_PERMUTATIONS is empty, skip.
	if os.Getenv("RUN_PERMUTATIONS") == "" {
		t.Skipf("Skipping %s because RUN_PERMUTATIONS is empty ", t.Name())
	}

	for i := range 3 {
		t.Run(fmt.Sprintf("iteration-%d", i+1), func(t *testing.T) {
			randomModifier := createRandomModifier()

			validatorID := thor.BytesToAddress([]byte("validator"))
			endorserID := thor.BytesToAddress([]byte("endorser"))
			delegationID := big.NewInt(1)

			epoch := HousekeepingInterval
			stakingPeriod := int(thor.LowStakingPeriod())

			addValidationMinBlk := randomModifier(&epoch)
			increaseValidationMinBlk := randomModifier(&epoch)
			signalExitMinBlock := randomModifier(&epoch)
			withdrawMinBlockCalc := convertNilBlock(signalExitMinBlock) + epoch + stakingPeriod + int(thor.CooldownPeriod())
			withdrawMinBlock := randomModifier(&withdrawMinBlockCalc)

			permManager := &permutationManager{
				actionConstructors: map[string]func(next ...Action) Action{
					"NewValidationAction": func(next ...Action) Action {
						return NewAddValidationAction(addValidationMinBlk, validatorID, endorserID, uint32(stakingPeriod), MinStakeVET, next...)
					},
					"NewIncreaseStakeAction": func(next ...Action) Action {
						return NewIncreaseStakeAction(increaseValidationMinBlk, validatorID, endorserID, MinStakeVET, next...)
					},
					"NewDecreaseStakeAction": func(next ...Action) Action {
						return NewDecreaseStakeAction(increaseValidationMinBlk, validatorID, endorserID, MinStakeVET, next...)
					},
					"NewSignalExitAction": func(next ...Action) Action {
						return NewSignalExitAction(signalExitMinBlock, validatorID, endorserID, next...)
					},
					"NewWithdrawAction": func(next ...Action) Action {
						return NewWithdrawAction(withdrawMinBlock, validatorID, endorserID, next...)
					},
					"NewAddDelegationAction": func(next ...Action) Action {
						return NewAddDelegationAction(addValidationMinBlk, validatorID, MinStakeVET, uint8(1), next...)
					},
					"NewSignalExitDelegationAction": func(next ...Action) Action {
						return NewSignalExitDelegationAction(addValidationMinBlk, delegationID, next...)
					},
					"NewWithdrawDelegationAction": func(next ...Action) Action {
						return NewWithdrawDelegationAction(withdrawMinBlock, delegationID, next...)
					},
				},
			}

			config := map[string]int{
				"NewValidationAction":           1,
				"NewIncreaseStakeAction":        2,
				"NewDecreaseStakeAction":        2,
				"NewSignalExitAction":           1,
				"NewWithdrawAction":             1,
				"NewAddDelegationAction":        1,
				"NewSignalExitDelegationAction": 1,
				"NewWithdrawDelegationAction":   1,
			}

			perms := permManager.GenerateAllPermutations(config)
			t.Logf("Total permutations to test: %d", len(perms))
			for i, order := range perms {
				action := permManager.InstantiateChain(order)
				staker := newTestStaker()
				require.NoError(t, RunWithResultTree(staker, action, 0))

				// Force garbage collection every 100 permutations
				if i%1000 == 0 {
					runtime.GC()
					runtime.GC()
					time.Sleep(time.Millisecond * 100) // Let GC finish
				}
			}
		})
		// Force aggressive garbage collection between iterations
		runtime.GC()
		runtime.GC()
		time.Sleep(time.Millisecond * 100) // Let GC finish
	}
}
