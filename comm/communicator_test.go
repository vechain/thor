// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package comm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameMajor(t *testing.T) {
	testCases := []struct {
		id         string
		appVersion string
		peerName   string
		expected   bool
	}{
		{
			id:         "only-major-same",
			appVersion: "2.0.0",
			peerName:   "thor/v2.1.1-88c7c86-release/linux/go1.21.9",
			expected:   true,
		},
		{
			id:         "only-major-different",
			appVersion: "2.1.1",
			peerName:   "thor/v1.1.1-88c7c86-release/linux/go1.21.9",
			expected:   false,
		},
		{
			id:         "app-version-empty",
			appVersion: "",
			peerName:   "thor/v1.1.1-88c7c86-release/linux/go1.21.9",
			expected:   true,
		},
		{
			id:         "exact-match",
			appVersion: "1.1.1",
			peerName:   "thor/v1.1.1-88c7c86-release/linux/go1.21.9",
			expected:   true,
		},
		{
			id: "bad-app-version",
			// bad app version, so accept any peer
			appVersion: "bad.bad.bad",
			peerName:   "thor/v1.1.1-88c7c86-release/linux/go1.21.9",
			expected:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			result := sameMajor(tc.appVersion, tc.peerName)
			assert.Equal(t, tc.expected, result)
		})
	}
}
