// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

// Test constants used across multiple test files
const (
	// Well-known VeChain addresses for testing
	VTHO_ADDRESS = "0x0000000000000000000000000000456E65726779"
	TEST_ADDRESS = "0x7567D83B7B8D80ADDCB281A71D54FC7B3364FFED"

	// Common VeChain event topics
	VTHO_TOPIC = "0xDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF"

	// Test database paths (can be overridden by flags)
	DEFAULT_TESTNET_DB = "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/logs-v2.db"
	DEFAULT_MAINNET_DB = "/Volumes/vechain/mainnet/data/instance-39627e6be7ec1b4a-v4/logs-v2.db"

	// Discovery sampling parameters
	DISCOVERY_SAMPLE_MODULO = 97 // Prime number for better distribution
	DISCOVERY_SEED          = 12345
)
