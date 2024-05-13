// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package datagen

import "fmt"

func RandHostPort() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d",
		RandIntN(254),
		RandIntN(254),
		RandIntN(254),
		RandIntN(254),
		RandIntN(10000))
}
