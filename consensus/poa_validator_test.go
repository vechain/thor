// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/scheduler"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestAuthorityCacheHandler_Success(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}
	mockForkConfig.HAYABUSA = 0

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
		{
			NodeMaster: thor.BytesToAddress([]byte("master2")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor2")),
			Identity:   thor.BytesToBytes32([]byte("identity2")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Staker.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("event1")),
							},
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_NilCandidates(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{}

	cacher := &poaCacher{nil, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_EmptyCandidates(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidates := scheduler.NewCandidates([]*authority.Candidate{})

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithAuthorityEvents(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Authority.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("candidate_added")),
								thor.BytesToBytes32(thor.BytesToAddress([]byte("new_candidate")).Bytes()),
							},
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithStakerEvents(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Staker.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("staker_event")),
							},
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithParamsEvents(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Params.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("params_event")),
							},
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithEndorsorTransfers(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Transfers: []*tx.Transfer{
						{
							Sender:    thor.BytesToAddress([]byte("endorsor1")),
							Recipient: thor.BytesToAddress([]byte("recipient")),
							Amount:    big.NewInt(1000),
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithAuthoritySetEvent(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: tx.Events{
						{
							Address: builtin.Authority.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("authority_set")),
							},
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	output, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
	assert.Nil(t, output)
}

func TestAuthorityCacheHandler_WithMultipleEvents(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: thor.BytesToAddress([]byte("contract1")),
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("event1")),
							},
						},
						{
							Address: thor.BytesToAddress([]byte("contract2")),
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("event2")),
							},
						},
					},
				},
			},
		},
	}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithNilReceipts(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, nil)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithEmptyReceipts(t *testing.T) {
	mockForkConfig := &thor.ForkConfig{}

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := scheduler.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{}

	cacher := &poaCacher{candidates, mockForkConfig}
	_, err := cacher.Handle(header, receipts)
	assert.NoError(t, err)
}
