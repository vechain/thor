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

	actionInfo := fmt.Sprintf("%s @ block %d (%d + %d)%s %s %s",
		action.Name(), executionBlock, parentBlock, blockAdvancement, amountDisplay, execResult, checkResult)

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
			log.Info("🏡 Housekeeping completed", "operations_count", housekeepingCount)
		}

		currentBlk += *mpb
		log.Debug("⬆️ Block advancement completed", "new_block", currentBlk)
	}

	log.Info("⚡️ Executing action", "action", action.Name(), "block", currentBlk)

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
		if err := RunnerWithResults(CloneStaker(s), next, currentBlk, results, clonedCtx); err != nil {
			log.Error("❌ Next action failed", "sequence", i+1, "action", next.Name(), "error", err)
			return err
		}
	}

	log.Debug("🎉 Action completed successfully", "action", action.Name())
	return nil
}

func TestPermutations(t *testing.T) {
	staker := newTestStaker()

	validatorID := thor.BytesToAddress([]byte("validator"))
	endorserID := thor.BytesToAddress([]byte("endorser"))
	lowStakingPeriod := int(thor.LowStakingPeriod())

	epoch := HousekeepingInterval
	plusNFun := func(base *int, increase int) *int {
		newBase := *base + increase
		return &newBase
	}

	//signalExitMinBlock := &epoch
	withDrawMinBlockCalc := lowStakingPeriod + epoch + int(thor.CooldownPeriod())
	//withDrawMinBlock := &withDrawMinBlockCalc

	// Compose the flow explicitly
	action := NewAddValidationAction(
		nil,
		validatorID,
		endorserID,
		thor.LowStakingPeriod(),
		MinStakeVET,
		//NewSignalExitAction(signalExitMinBlock, validatorID, endorserID,
		//	NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
		//),
		//NewIncreaseStakeAction(&epoch, validatorID, endorserID, MinStakeVET,
		//	NewIncreaseStakeAction(plusNFun(&epoch, 2), validatorID, endorserID, MinStakeVET,
		//		NewIncreaseStakeAction(plusNFun(&epoch, 3), validatorID, endorserID, MinStakeVET,
		//			NewSignalExitAction(plusNFun(&epoch, 5), validatorID, endorserID,
		//				NewWithdrawAction(&withDrawMinBlockCalc, validatorID, endorserID),
		//			),
		//		),
		//	),
		//),
		//NewAddDelegationAction(&epoch, validatorID, MinStakeVET, uint8(1),
		//	NewSignalExitDelegationAction(plusNFun(&lowStakingPeriod, 2), big.NewInt(1),
		//		NewWithdrawDelegationAction(&withDrawMinBlockCalc, big.NewInt(1)),
		//	),
		//),

		NewAddDelegationAction(&epoch, validatorID, MinStakeVET, uint8(1),
			NewSignalExitAction(plusNFun(&epoch, 2), validatorID, endorserID,
				NewSignalExitDelegationAction(plusNFun(&lowStakingPeriod, 2), big.NewInt(1),
					NewWithdrawDelegationAction(&withDrawMinBlockCalc, big.NewInt(1),
						NewWithdrawAction(&withDrawMinBlockCalc, validatorID, endorserID),
					),
				),
			),
		),

		//NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
		//NewIncreaseStakeAction(withDrawMinBlock, validatorID, endorserID, MinStakeVET,
		//	NewDecreaseStakeAction(withDrawMinBlock, validatorID, endorserID, MinStakeVET,
		//		NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
		//	),
		//	NewSignalExitAction(withDrawMinBlock, validatorID, endorserID,
		//		NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
		//	),
		//),
		//NewDecreaseStakeAction(withDrawMinBlock, validatorID, endorserID, MinStakeVET,
		//	NewDecreaseStakeAction(withDrawMinBlock, validatorID, endorserID, MaxStakeVET*2),
		//),
		//NewAddDelegationAction(originalModifier(nil), validatorID, MinStakeVET, uint8(1),
		//	NewSignalExitDelegationAction(originalModifier(&lowStakingPeriod), big.NewInt(1)),
		//	NewSignalExitAction(withDrawMinBlock, validatorID, endorserID,
		//		NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
		//	),
		//),
		//NewAddDelegationAction(originalModifier(nil), validatorID, MinStakeVET, uint8(1),
		//	NewSignalExitDelegationAction(originalModifier(&lowStakingPeriod), big.NewInt(1),
		//		NewWithdrawAction(originalModifier(&lowStakingPeriod), validatorID, endorserID),
		//	),
		//	NewSignalExitAction(withDrawMinBlock, validatorID, endorserID,
		//		NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
		//	),
		//),
	)

	require.NoError(t, RunWithResultTree(staker, action, 0))
}

func TestRandomPermutations(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	for i := range 1_000 {
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
			action := NewAddValidationAction(
				randomModifier(nil),
				validatorID,
				endorserID,
				thor.LowStakingPeriod(),
				MinStakeVET,
				NewSignalExitAction(signalExitMinBlock, validatorID, endorserID,
					NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
				),
				NewWithdrawAction(withDrawMinBlock, validatorID, endorserID),
			)

			require.NoError(t, RunWithResultTree(staker, action, 0))
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

// ----------------------
// Amount Calculation Helpers
// ----------------------

// calculateExpectedValidationWithdrawAmount calculates the expected amount for validation withdrawals
func calculateExpectedValidationWithdrawAmount(ctx *ExecutionContext, currentBlock int) uint64 {
	if ctx == nil {
		return 0
	}

	// Rule 1: If no SignalExit has been called, can only withdraw adjustments made in same epoch
	if ctx.SignalExitBlock == nil {
		withdrawableAmt := int64(0)
		for _, adjustment := range ctx.StakeAdjustments {
			if IsSameEpoch(currentBlock, adjustment.Block) {
				withdrawableAmt += adjustment.Amount
			}
		}

		if withdrawableAmt > 0 {
			return uint64(withdrawableAmt)
		}
		return 0
	}

	// Rule 2: SignalExit is set - two-part calculation
	if ctx.ValidationStartBlock == nil {
		return 0
	}

	// Part A: Same-epoch adjustments (always allowed)
	sameEpochAdjustments := int64(0)
	for _, adjustment := range ctx.StakeAdjustments {
		if IsSameEpoch(currentBlock, adjustment.Block) {
			sameEpochAdjustments += adjustment.Amount
		}
	}
	// Part B: Check if enough time has passed to withdraw initial stake + all adjustments
	signalExitBlock := *ctx.SignalExitBlock
	stakingPeriod := int(thor.LowStakingPeriod())
	cooldownPeriod := int(thor.CooldownPeriod())

	// Calculate time to next housekeeping from signal exit
	timeToNextHousekeeping := HousekeepingInterval - (signalExitBlock % HousekeepingInterval)
	if timeToNextHousekeeping == HousekeepingInterval {
		timeToNextHousekeeping = 0 // Already at housekeeping boundary
	}

	requiredBlock := signalExitBlock + timeToNextHousekeeping + stakingPeriod + cooldownPeriod

	// Part C: Adjustments made before SignalExit are immediately withdrawable
	adjustmentsBeforeSignalExit := int64(0)
	for _, adj := range ctx.StakeAdjustments {
		if adj.Block <= signalExitBlock {
			adjustmentsBeforeSignalExit += adj.Amount
		}
	}

	if currentBlock >= requiredBlock {
		// Can withdraw initial stake + all adjustments
		totalStake := int64(ctx.InitialStake)
		for _, adj := range ctx.StakeAdjustments {
			totalStake += adj.Amount
		}

		if totalStake > 0 {
			return uint64(totalStake)
		}
	}

	// Return the maximum of same-epoch adjustments or adjustments before SignalExit
	withdrawableAmount := sameEpochAdjustments
	if adjustmentsBeforeSignalExit > withdrawableAmount {
		withdrawableAmount = adjustmentsBeforeSignalExit
	}

	if withdrawableAmount > 0 {
		return uint64(withdrawableAmount)
	}

	return 0
}

// calculateExpectedDelegationWithdrawAmount calculates the expected amount for delegation withdrawals
func calculateExpectedDelegationWithdrawAmount(ctx *ExecutionContext, currentBlock int) uint64 {
	if ctx == nil {
		return 0
	}

	// Rule 1: If both DelegationExitBlock == nil AND SignalExitBlock == nil → return 0 (no signal)
	if ctx.DelegationExitBlock == nil && ctx.SignalExitBlock == nil {
		return 0
	}

	// Rule 2: If either SignalExitBlock OR DelegationExitBlock are set

	// Rule 2a: If DelegationExitBlock was done in same epoch as AddDelegation → can exit
	if ctx.DelegationExitBlock != nil && ctx.DelegationStartBlock != nil {
		if IsSameEpoch(*ctx.DelegationExitBlock, *ctx.DelegationStartBlock) {
			return ctx.InitialDelegationStake
		}
	}

	// Rule 2b: If the older of (SignalExitBlock, DelegationExitBlock) was done at least a staking period ago → can exit
	var olderExitBlock int
	hasExitBlock := false

	if ctx.SignalExitBlock != nil && ctx.DelegationExitBlock != nil {
		// Both exist, use the older (smaller) one
		if *ctx.SignalExitBlock < *ctx.DelegationExitBlock {
			olderExitBlock = *ctx.SignalExitBlock
		} else {
			olderExitBlock = *ctx.DelegationExitBlock
		}
		hasExitBlock = true
	} else if ctx.SignalExitBlock != nil {
		// Only SignalExitBlock exists
		olderExitBlock = *ctx.SignalExitBlock
		hasExitBlock = true
	} else if ctx.DelegationExitBlock != nil {
		// Only DelegationExitBlock exists
		olderExitBlock = *ctx.DelegationExitBlock
		hasExitBlock = true
	}

	if hasExitBlock {
		stakingPeriod := int(thor.LowStakingPeriod())

		// Calculate time to next housekeeping from the older exit block
		timeToNextHousekeeping := HousekeepingInterval - (olderExitBlock % HousekeepingInterval)
		if timeToNextHousekeeping == HousekeepingInterval {
			timeToNextHousekeeping = 0 // Already at housekeeping boundary
		}

		requiredBlock := olderExitBlock + timeToNextHousekeeping + stakingPeriod

		if currentBlock >= requiredBlock {
			// Enough time has passed, can withdraw delegation amount
			return ctx.InitialDelegationStake
		}
	}

	// No conditions met for withdrawal
	return 0
}

// IsSameEpoch returns true if both blocks are within the same housekeeping epoch.
// Epochs are defined as [1..interval], [interval+1..2*interval], etc.
func IsSameEpoch(a, b int) bool {
	return (a-1)/HousekeepingInterval == (b-1)/HousekeepingInterval
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
