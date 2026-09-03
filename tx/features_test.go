// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeatures(t *testing.T) {
	var f Features

	assert.Zero(t, f)
	assert.False(t, f.IsDelegated())

	f.SetDelegated(true)
	assert.True(t, f.IsDelegated())

	f.SetDelegated(false)
	assert.False(t, f.IsDelegated())

	f.SetDelegated(false)
	assert.False(t, f.IsDelegated())
}
