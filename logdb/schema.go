// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

// create a table for events
const (
	configTableSchema = `CREATE TABLE IF NOT EXISTS config (
	key CHAR(20) PRIMARY KEY,
	value BLOB
);`

	eventTableSchema = `CREATE TABLE IF NOT EXISTS event (
	blockNumber INTEGER,
	eventIndex INTEGER,
	blockID	BLOB(32),
	blockTime INTEGER,
	txID BLOB(32),
	txOrigin BLOB(20),
	clauseIndex INTEGER,
	address BLOB(20),	
	topic0 BLOB(32),
	topic1 BLOB(32),
	topic2 BLOB(32),
	topic3 BLOB(32),
	topic4 BLOB(32),
	data BLOB
);

CREATE UNIQUE INDEX IF NOT EXISTS event_i0 ON event(blockNumber, eventIndex);
CREATE INDEX IF NOT EXISTS event_i1 ON event(address, blockNumber, eventIndex);
CREATE INDEX IF NOT EXISTS event_i2 ON event(topic0, blockNumber, eventIndex);
CREATE INDEX IF NOT EXISTS event_i3 ON event(topic1, blockNumber, eventIndex);
CREATE INDEX IF NOT EXISTS event_i4 ON event(topic2, blockNumber, eventIndex);
CREATE INDEX IF NOT EXISTS event_i5 ON event(topic3, blockNumber, eventIndex);
CREATE INDEX IF NOT EXISTS event_i6 ON event(topic4, blockNumber, eventIndex);`

	// create a table for transfer
	transferTableSchema = `CREATE TABLE IF NOT EXISTS transfer (
	blockNumber INTEGER,
	transferIndex INTEGER,
	blockID	BLOB(32),
	blockTime INTEGER,
	txID BLOB(32),
	txOrigin BLOB(20),
	clauseIndex INTEGER,
	sender BLOB(20),
	recipient BLOB(20),
	amount BLOB(32)
);

CREATE UNIQUE INDEX IF NOT EXISTS transfer_i0 ON transfer(blockNumber, transferIndex);
CREATE INDEX IF NOT EXISTS transfer_i1 ON transfer(sender, blockNumber, transferIndex);
CREATE INDEX IF NOT EXISTS transfer_i2 ON transfer(recipient, blockNumber, transferIndex);`
)
