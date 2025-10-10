// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import (
	"testing"
)

func TestGoes(t *testing.T) {
	var g Goes
	g.Go(func() {})
	g.Go(func() {})
	g.Wait()

	<-g.Done()
}
