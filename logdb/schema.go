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
);`

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
);`
)
