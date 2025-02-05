// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package utils

import (
	"fmt"
	"math"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

func TestParseRevision(t *testing.T) {
	testCases := []struct {
		revision string
		err      error
		expected *Revision
	}{
		{
			revision: "",
			err:      nil,
			expected: &Revision{revBest},
		},
		{
			revision: "1234",
			err:      nil,
			expected: &Revision{uint32(1234)},
		},
		{
			revision: "best",
			err:      nil,
			expected: &Revision{revBest},
		},
		{
			revision: "justified",
			err:      nil,
			expected: &Revision{revJustified},
		},
		{
			revision: "finalized",
			err:      nil,
			expected: &Revision{revFinalized},
		}, {
			revision: "next",
			err:      nil,
			expected: &Revision{revNext},
		},
		{
			revision: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			err:      nil,
			expected: &Revision{thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")},
		},
		{
			revision: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdzz",
			err:      errors.New("encoding/hex: invalid byte: U+007A 'z'"),
			expected: nil,
		},
		{
			revision: "1234567890abcdef1234567890abcde",
			err:      errors.New("strconv.ParseUint: parsing \"1234567890abcdef1234567890abcde\": invalid syntax"),
			expected: nil,
		},
		{
			revision: fmt.Sprintf("%v", uint64(math.MaxUint64)),
			err:      errors.New("block number out of max uint32"),
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.revision, func(t *testing.T) {
			result, err := ParseRevision(tc.revision, true)
			if tc.err != nil {
				assert.Equal(t, tc.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestAllowNext(t *testing.T) {
	_, err := ParseRevision("next", false)
	assert.Error(t, err, "invalid revision: next is not allowed")

	_, err = ParseRevision("next", true)
	assert.Nil(t, err)
	_, err = ParseRevision("finalized", false)
	assert.Nil(t, err)
}

func TestGetSummary(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	// Test cases
	testCases := []struct {
		name     string
		revision *Revision
		err      error
	}{
		{
			name:     "best",
			revision: &Revision{revBest},
			err:      nil,
		},
		{
			name:     "1234",
			revision: &Revision{uint32(1234)},
			err:      errors.New("not found"),
		},
		{
			name:     "justified",
			revision: &Revision{revJustified},
			err:      nil,
		},
		{
			name:     "finalized",
			revision: &Revision{revFinalized},
			err:      nil,
		},
		{
			name:     "0x00000000c05a20fbca2bf6ae3affba6af4a74b800b585bf7a4988aba7aea69f6",
			revision: &Revision{thor.MustParseBytes32("0x00000000c05a20fbca2bf6ae3affba6af4a74b800b585bf7a4988aba7aea69f6")},
			err:      nil,
		},
		{
			name:     "next",
			revision: &Revision{revNext},
			err:      errors.New("invalid revision"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary, err := GetSummary(tc.revision, thorChain.Repo(), thorChain.Engine())
			if tc.err != nil {
				assert.Equal(t, tc.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.NotNil(t, summary)
			}
		})
	}
}

func TestGetSummaryAndState(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	b := thorChain.GenesisBlock()

	summary, _, err := GetSummaryAndState(&Revision{revBest}, thorChain.Repo(), thorChain.Engine(), thorChain.Stater())
	assert.Nil(t, err)
	assert.Equal(t, summary.Header.Number(), b.Header().Number())
	assert.Equal(t, summary.Header.Timestamp(), b.Header().Timestamp())

	summary, _, err = GetSummaryAndState(&Revision{revNext}, thorChain.Repo(), thorChain.Engine(), thorChain.Stater())
	assert.Nil(t, err)
	assert.Equal(t, summary.Header.Number(), b.Header().Number()+1)
	assert.Equal(t, summary.Header.Timestamp(), b.Header().Timestamp()+thor.BlockInterval)

	signer, err := summary.Header.Signer()
	assert.NotNil(t, err)
	assert.True(t, signer.IsZero())
}
