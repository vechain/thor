// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"testing"
)

type mockedDerivableList struct {
	n       int
	content []byte
}

func (l *mockedDerivableList) Len() int { return l.n }

func (l *mockedDerivableList) EncodeIndex(i int) []byte { return l.content }

func BenchmarkDeriveRoot(b *testing.B) {
	list := mockedDerivableList{
		n:       100,
		content: make([]byte, 32),
	}
	for i := 0; i < b.N; i++ {
		DeriveRoot(&list)
	}
}
