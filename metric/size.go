// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metric

import (
	"fmt"
	"io"
)

// StorageSize describes storage size in bytes.
type StorageSize int64

func (ss StorageSize) String() string {
	if ss > 1000000000 {
		return fmt.Sprintf("%.2f gB", float64(ss)/1000000000)
	} else if ss > 1000000 {
		return fmt.Sprintf("%.2f mB", float64(ss)/1000000)
	} else if ss > 1000 {
		return fmt.Sprintf("%.2f kB", float64(ss)/1000)
	}
	return fmt.Sprintf("%d B", ss)
}

// Int64 returns int64 value.
func (ss StorageSize) Int64() int64 {
	return int64(ss)
}

// Write implements io.Writer, so it can be passed into function
// that accepts io.Writer to count written bytes.
func (ss *StorageSize) Write(b []byte) (int, error) {
	n := len(b)
	*ss += StorageSize(n)
	return n, nil
}

var _ io.Writer = (*StorageSize)(nil)
