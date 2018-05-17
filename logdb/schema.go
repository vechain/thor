// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

// create a table for events
const (
	eventTableSchema = `CREATE TABLE IF NOT EXISTS event (
	blockID	BLOB(32),
	eventIndex INTEGER,
	blockNumber INTEGER,
	blockTime INTEGER,
	txID BLOB(32),
	txOrigin BLOB(20),
	address BLOB(20),	
	topic0 BLOB(32),
	topic1 BLOB(32),
	topic2 BLOB(32),
	topic3 BLOB(32),
	topic4 BLOB(32),
	data BLOB
);

CREATE UNIQUE INDEX IF NOT EXISTS prim ON event(blockID, eventIndex);

CREATE INDEX IF NOT EXISTS blockNumberIndex ON event(blockNumber);
CREATE INDEX IF NOT EXISTS blockTimeIndex ON event(blockTime);
CREATE INDEX IF NOT EXISTS addressIndex ON event(address);
CREATE INDEX IF NOT EXISTS topicIndex0 ON event(topic0);
CREATE INDEX IF NOT EXISTS topicIndex1 ON event(topic1);
CREATE INDEX IF NOT EXISTS topicIndex2 ON event(topic2);
CREATE INDEX IF NOT EXISTS topicIndex3 ON event(topic3);
CREATE INDEX IF NOT EXISTS topicIndex4 ON event(topic4);`

	// create a table for transfer
	transferTableSchema = `CREATE TABLE IF NOT EXISTS transfer (
	blockID	BLOB(32),
	transferIndex INTEGER,
	blockNumber INTEGER,
	blockTime INTEGER,
	txID BLOB(32),
	txOrigin BLOB(20),
	sender BLOB(20),
	recipient BLOB(20),
	amount BLOB
);

CREATE UNIQUE INDEX IF NOT EXISTS prim ON transfer(blockID, transferIndex);

CREATE INDEX IF NOT EXISTS blockNumberIndex ON transfer(blockNumber);
CREATE INDEX IF NOT EXISTS blockTimeIndex ON transfer(blockTime);
CREATE INDEX IF NOT EXISTS senderIndex ON transfer(sender);
CREATE INDEX IF NOT EXISTS recipientIndex ON transfer(recipient);`
)
