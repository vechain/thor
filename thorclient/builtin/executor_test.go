// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

	approver1 := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)
	approver2 := bind.NewSigner(genesis.DevAccounts()[1].PrivateKey)
	approver3 := bind.NewSigner(genesis.DevAccounts()[2].PrivateKey)

	newApprover := bind.NewSigner(genesis.DevAccounts()[3].PrivateKey)

	// Approvers
	approver, err := executor.Approvers(approver1.Address())
	require.NoError(t, err)
	require.True(t, approver.InPower)

	// ApproverCount
	approverCount, err := executor.ApproverCount()
	require.NoError(t, err)
	require.Equal(t, uint8(3), approverCount)

	// Propose - Add another approver
	// approver1,
	addApproverClause, err := executor.AddApprover(newApprover.Address(), datagen.RandomHash()).Clause().Build()
	require.NoError(t, err)
	receipt, _, err := executor.Propose(*addApproverClause.To(), addApproverClause.Data()).Send().WithSigner(approver1).WithOptions(txOpts()).Receipt(txContext(t))
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
	receipt, _, err = executor.Approve(proposals[0].ProposalID).Send().WithSigner(approver1).WithOptions(txOpts()).Receipt(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)
	receipt, _, err = executor.Approve(proposals[0].ProposalID).Send().WithSigner(approver2).WithOptions(txOpts()).Receipt(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)
	receipt, _, err = executor.Approve(proposals[0].ProposalID).Send().WithSigner(approver3).WithOptions(txOpts()).Receipt(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// Execute
	receipt, _, err = executor.Execute(proposals[0].ProposalID).Send().WithSigner(approver1).WithOptions(txOpts()).Receipt(txContext(t))
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
	_, err = executor.RevokeApprover(newApprover.Address()).Clause().Build()
	require.NoError(t, err)

	// AttachVotingContract - Clause only
	_, err = executor.AttachVotingContract(thor.Address{}).Clause().Build()
	require.NoError(t, err)

	// DetachVotingContract - Clause only
	_, err = executor.DetachVotingContract(thor.Address{}).Clause().Build()
	require.NoError(t, err)
}
