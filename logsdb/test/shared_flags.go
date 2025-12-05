// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import "flag"

// Shared flag definitions to prevent redefinition conflicts
var (
	SqliteDbPath  = flag.String("sqliteDbPath", "", "Path to the SQLite database file (used for both benchmarks and discovery)")
	PebblePath    = flag.String("pebblePath", "", "Path to the Pebble database directory")
	DiscoveryMode = flag.String("discoveryMode", "fast", "Discovery mode: fast (range-based sampling) or full (complete statistical sampling)")
	Verbose       = flag.Bool("verbose", false, "Enable verbose logging output")
)
