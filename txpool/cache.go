// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import Cache "github.com/vechain/thor/cache"

type cache interface {
	Set(key, value interface{})
	Get(key interface{}) (interface{}, bool)
	Remove(key interface{}) bool
	Len() int
	ForEach(cb func(*Cache.Entry) bool) bool
}
