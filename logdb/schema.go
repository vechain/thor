// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

const (
	refTableScheme = `CREATE TABLE IF NOT EXISTS ref (
	id INTEGER PRIMARY KEY NOT NULL,
	data BLOB NOT NULL UNIQUE
);`

	// creates events table
	eventTableSchema = `CREATE TABLE IF NOT EXISTS event (
	seq INTEGER PRIMARY KEY NOT NULL,
	blockID	INTEGER NOT NULL,
	blockTime INTEGER NOT NULL,
	txID INTEGER NOT NULL,
	txOrigin INTEGER NOT NULL,
	clauseIndex INTEGER NOT NULL,
	address INTEGER NOT NULL,
	topic0 INTEGER,
	topic1 INTEGER,
	topic2 INTEGER,
	topic3 INTEGER,
	topic4 INTEGER,
	data BLOB
);

CREATE INDEX IF NOT EXISTS event_i0 ON event(address);
CREATE INDEX IF NOT EXISTS event_i1 ON event(topic0, address);
CREATE INDEX IF NOT EXISTS event_i2 ON event(topic1, topic0, address) WHERE topic1 IS NOT NULL;
CREATE INDEX IF NOT EXISTS event_i3 ON event(topic2, topic0, address) WHERE topic2 IS NOT NULL;
CREATE INDEX IF NOT EXISTS event_i4 ON event(topic3, topic0, address) WHERE topic3 IS NOT NULL;`

	// create transfers table
	transferTableSchema = `CREATE TABLE IF NOT EXISTS transfer (
	seq INTEGER PRIMARY KEY NOT NULL,
	blockID	INTEGER NOT NULL,
	blockTime INTEGER NOT NULL,
	txID INTEGER NOT NULL,
	txOrigin INTEGER NOT NULL,
	clauseIndex INTEGER NOT NULL,
	sender INTEGER NOT NULL,
	recipient INTEGER NOT NULL,
	amount BLOB(32)
);

CREATE INDEX IF NOT EXISTS transfer_i0 ON transfer(txOrigin);
CREATE INDEX IF NOT EXISTS transfer_i1 ON transfer(sender);
CREATE INDEX IF NOT EXISTS transfer_i2 ON transfer(recipient);`
)
