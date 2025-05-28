package builtin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestExecutor(t *testing.T) {
	_, client := newChain(t, true)

	executor, err := NewExecutor(client)
	require.NoError(t, err)

	approver1 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[0].PrivateKey)
	approver2 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[1].PrivateKey)
	approver3 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[2].PrivateKey)

	newApprover := (*bind.PrivateKeySigner)(genesis.DevAccounts()[3].PrivateKey)

	// Approvers
	approver, err := executor.Approvers(approver1.Address())
	require.NoError(t, err)
	require.True(t, approver.InPower)

	// ApproverCount
	approverCount, err := executor.ApproverCount()
	require.NoError(t, err)
	require.Equal(t, uint8(3), approverCount)

	// Propose - Add another approver
	addApproverClause, err := executor.AddApprover(approver1, newApprover.Address(), datagen.RandomHash()).Clause()
	require.NoError(t, err)
	receipt, _, err := executor.Propose(approver1, *addApproverClause.To(), addApproverClause.Data()).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// Filter Proposals
	proposals, err := executor.FilterProposals(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, proposals, 1)

	// Proposal
	proposal, err := executor.Proposals(proposals[0].ProposalID)
	require.NoError(t, err)
	require.Equal(t, addApproverClause.To(), &proposal.Target)
	require.Equal(t, addApproverClause.Data(), proposal.Data)
	require.False(t, proposal.Executed)

	// Approve
	receipt, _, err = executor.Approve(approver1, proposals[0].ProposalID).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)
	receipt, _, err = executor.Approve(approver2, proposals[0].ProposalID).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)
	receipt, _, err = executor.Approve(approver3, proposals[0].ProposalID).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// Execute
	receipt, _, err = executor.Execute(approver1, proposals[0].ProposalID).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// Proposal
	proposal, err = executor.Proposals(proposals[0].ProposalID)
	require.NoError(t, err)
	require.True(t, proposal.Executed)
	require.Equal(t, uint8(3), proposal.ApprovalCount)

	// ApproverCount
	approverCount, err = executor.ApproverCount()
	require.NoError(t, err)
	require.Equal(t, uint8(4), approverCount)

	// Check if new approver is added
	newApproverInfo, err := executor.Approvers(newApprover.Address())
	require.NoError(t, err)
	require.True(t, newApproverInfo.InPower)

	// RevokeApprover - Clause only
	_, err = executor.RevokeApprover(approver1, newApprover.Address()).Clause()
	require.NoError(t, err)

	// AttachVotingContract - Clause only
	_, err = executor.AttachVotingContract(approver1, thor.Address{}).Clause()
	require.NoError(t, err)

	// DetachVotingContract - Clause only
	_, err = executor.DetachVotingContract(approver1, thor.Address{}).Clause()
	require.NoError(t, err)
}
