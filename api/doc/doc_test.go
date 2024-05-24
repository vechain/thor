// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package doc

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	validVersion := regexp.MustCompile(`^\d+(\.\d+){2}$`)

	assert.True(t, validVersion.Match([]byte(Version())))
}
