// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapSlice(t *testing.T) {
	require.Nil(t, MapSlice([]int(nil), strconv.Itoa))
	require.Equal(t, []string{"1", "2"}, MapSlice([]int{1, 2}, strconv.Itoa))
}
