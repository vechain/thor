package staker

import (
	"fmt"
	"log/slog"
	"math/big"
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
}

type StakeAdjustment struct {
	Block  int
	Amount int64 // positive for increase, negative for decrease
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

	return clone
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

// PrintActionTreeWithResults prints a tree visualization with execution results
func PrintActionTreeWithResults(action Action, results map[string]*ActionResult) {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))

	log.Debug("📊 Action Execution Results:")
	printActionTreeWithResultsHelper(action, "", true, 0, results, log)
	log.Debug("🏁 End of execution results")
}

func printActionTreeWithResultsHelper(action Action, prefix string, isLast bool, parentBlock int, results map[string]*ActionResult, log *slog.Logger) {
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

	// Look up results for this action
	actionKey := fmt.Sprintf("%s@%d", action.Name(), executionBlock)
	result, found := results[actionKey]

	// Format result indicators
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
	if executionBlock > stakingPeriod {
		excessBlocks := executionBlock - stakingPeriod
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
		printActionTreeWithResultsHelper(nextAction, childPrefix, isLastChild, executionBlock, results, log)
	}
}

// RunWithResultTree executes actions and prints a tree with results
func RunWithResultTree(s *testStaker, action Action, currentBlk int) error {
	results := make(map[string]*ActionResult)
	ctx := &ExecutionContext{}
	err := RunnerWithResults(s, action, currentBlk, results, ctx)
	PrintActionTreeWithResults(action, results)
	return err
}

// RunnerWithResults executes an action, runs its checks, captures results, then recurses on Next().
func RunnerWithResults(s *testStaker, action Action, currentBlk int, results map[string]*ActionResult, ctx *ExecutionContext) error {
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
			log.Debug("🏡 Housekeeping completed", "operations_count", housekeepingCount)
		}

		currentBlk += *mpb
		log.Debug("⬆️ Block advancement completed", "new_block", currentBlk)
	}

	log.Debug("⚡️ Executing action", "action", action.Name(), "block", currentBlk)

	// Execute and check with context
	executionErr := action.Execute(ctx, s, currentBlk)
	checkErr := action.Check(ctx, s, currentBlk)

	// Create a unique key for this action execution
	actionKey := fmt.Sprintf("%s@%d", action.Name(), currentBlk)
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
		clonedCtx := ctx.Clone()
		if err := RunnerWithResults(cloneStaker(s), next, currentBlk, results, clonedCtx); err != nil {
			log.Error("❌ Next action failed", "sequence", i+1, "action", next.Name(), "error", err)
			return err
		}
	}

	log.Debug("🎉 Action completed successfully", "action", action.Name())
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
	base := 0
	if i != nil {
		base = *i
	}
	return base
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
func (pm *permutationManager) GenerateAllPermutations(config map[string]int) [][]string {
	const headKey = "NewValidationAction"
	countHead := config[headKey]
	if countHead < 1 {
		return nil // no permutations allowed if no NewValidationAction
	}
	// Build action list excluding ONE head action to be inserted at front.
	var baseList []string
	for name, count := range config {
		c := count
		if name == headKey {
			c-- // We've reserved one for the head
		}
		for i := 0; i < c; i++ {
			baseList = append(baseList, name)
		}
	}
	var results [][]string
	var permute func([]string, int)
	permute = func(arr []string, l int) {
		if l == len(arr)-1 {
			p := make([]string, len(arr))
			copy(p, arr)
			// Always prepend headKey
			results = append(results, append([]string{headKey}, p...))
			return
		}
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
	if len(baseList) == 0 {
		// Just [headKey] is the only permutation
		results = [][]string{{headKey}}
	} else {
		permute(baseList, 0)
	}
	return results
}

// Given a permutation (list of action names), chain the real Action objects using hardcoded arguments.
func (pm *permutationManager) InstantiateChain(order []string) Action {
	if len(order) == 0 {
		return nil
	}
	// Remove these unused variables (declared but not used):
	// validatorID, endorserID, lowStakingPeriod, minStake, maxStake, withDrawMinBlock, beneficiary, delegationID
	// For this example, use nil for minParentBlocksRequired etc.

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

	// Block advancement values to match the scenario
	addValidationBlocks := 32
	increaseStakeBlocks := 89
	addDelegationBlocks := 32
	signalExitBlocks := 50
	withdrawBlocks := 69138 // This will result in block 69519 (381 + 69138)
	signalExitDelegationBlocks := 32
	withdrawDelegationBlocks := 69138

	action := NewAddValidationAction(
		&addValidationBlocks,
		validatorID,
		endorserID,
		thor.LowStakingPeriod(),
		MinStakeVET,
		NewIncreaseStakeAction(&increaseStakeBlocks, validatorID, endorserID, MinStakeVET,
			NewIncreaseStakeAction(&increaseStakeBlocks, validatorID, endorserID, MinStakeVET,
				NewIncreaseStakeAction(&increaseStakeBlocks, validatorID, endorserID, MinStakeVET,
					NewAddDelegationAction(&addDelegationBlocks, validatorID, MinStakeVET, uint8(1),
						NewSignalExitAction(&signalExitBlocks, validatorID, endorserID,
							NewWithdrawAction(&withdrawBlocks, validatorID, endorserID,
								NewSignalExitDelegationAction(&signalExitDelegationBlocks, delegationID,
									NewWithdrawDelegationAction(&withdrawDelegationBlocks, delegationID),
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
	rand.Seed(time.Now().UnixNano())

	for i := range 1 {
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
				"NewIncreaseStakeAction":        3,
				"NewSignalExitAction":           1,
				"NewWithdrawAction":             1,
				"NewAddDelegationAction":        1,
				"NewSignalExitDelegationAction": 1,
				"NewWithdrawDelegationAction":   1,
			}

			perms := permManager.GenerateAllPermutations(config)
			for _, order := range perms {
				action := permManager.InstantiateChain(order)
				staker := newTestStaker()
				require.NoError(t, RunWithResultTree(staker, action, 0))
			}
		})
	}
}
