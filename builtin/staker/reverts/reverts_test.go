// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package reverts

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Reverts(t *testing.T) {
	revert := New("test")
	assert.Equal(t, "test", revert.message)
	assert.Equal(t, revert.Error(), revert.message)

	assert.True(t, IsRevertErr(revert))
	assert.False(t, IsRevertErr(nil))
	assert.False(t, IsRevertErr(fmt.Errorf("test")))
	assert.False(t, IsRevertErr(big.NewInt(0)))
}
