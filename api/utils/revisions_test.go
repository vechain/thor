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
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func TestParseRevision(t *testing.T) {
	testCases := []struct {
		revision string
		err      error
		expected Revision
	}{
		{
			revision: "",
			err:      nil,
			expected: nil,
		},
		{
			revision: "1234",
			err:      nil,
			expected: uint32(1234),
		},
		{
			revision: "best",
			err:      nil,
			expected: nil,
		},
		{
			revision: "finalized",
			err:      nil,
			expected: "finalized",
		},
		{
			revision: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			err:      nil,
			expected: thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
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
			result, err := ParseRevision(tc.revision)
			if tc.err != nil {
				assert.Equal(t, tc.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestGetSummary(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()
	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)
	bft := solo.NewBFTEngine(repo)

	// Test cases
	testCases := []struct {
		revision Revision
		err      error
	}{
		{
			revision: nil,
			err:      nil,
		},
		{
			revision: uint32(1234),
			err:      errors.New("not found"),
		},
		{
			revision: "finalized",
			err:      nil,
		},
		{
			revision: thor.MustParseBytes32("0x00000000c05a20fbca2bf6ae3affba6af4a74b800b585bf7a4988aba7aea69f6"),
			err:      nil,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc.revision), func(t *testing.T) {
			summary, err := GetSummary(tc.revision, repo, bft)
			if tc.err != nil {
				assert.Equal(t, tc.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.NotNil(t, summary)
			}
		})
	}
}

func TestParseCodeCallRevision(t *testing.T) {
	testCases := []struct {
		revision string
		expected Revision
		isNext   bool
	}{
		{
			revision: "",
			expected: nil,
			isNext:   true,
		},
		{
			revision: "1234",
			expected: uint32(1234),
			isNext:   false,
		},
		{
			revision: "best",
			expected: nil,
			isNext:   true,
		},
		{
			revision: "next",
			expected: nil,
			isNext:   true,
		},
		{
			revision: "0x00000000c05a20fbca2bf6ae3affba6af4a74b800b585bf7a4988aba7aea69f6",
			expected: thor.MustParseBytes32("0x00000000c05a20fbca2bf6ae3affba6af4a74b800b585bf7a4988aba7aea69f6"),
			isNext:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.revision, func(t *testing.T) {
			result, isNext, err := ParseCodeCallRevision(tc.revision)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.isNext, isNext)
		})
	}
}
