// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package state manages the main accounts trie.
// It follows the flow as bellow:
//
//            o
//            |
//   [ revertable state ]
//            |
//     [ stacked map ] -> [ journal ] -> [ playback(staging) ] -> [ updated trie ]
//            |
//      [ trie cache ]
//            |
//     [ read-only trie ]
//
// It's much simpler than Ethereum's statedb.
// An important difference with statedb is the logic of account suicide.
// TODO: explain more
//
package state
