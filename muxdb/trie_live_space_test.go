// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTrieLiveSpace(t *testing.T) {
	db := newMemDB()

	s, err := newTrieLiveSpace(db)
	assert.Nil(t, err)

	assert.Equal(t, trieSpaceA, s.Active())
	assert.Equal(t, trieSpaceB, s.Stale())

	assert.Nil(t, s.Switch())

	assert.Equal(t, trieSpaceB, s.Active())
	assert.Equal(t, trieSpaceA, s.Stale())

	// create upon existing db
	s, err = newTrieLiveSpace(db)
	assert.Nil(t, err)

	assert.Equal(t, trieSpaceB, s.Active())
	assert.Equal(t, trieSpaceA, s.Stale())
}
