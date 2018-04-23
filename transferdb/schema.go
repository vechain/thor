package transferdb

// create a table for transfer
const transferTableSchema = `
CREATE TABLE IF NOT EXISTS transfer (
	blockID	BLOB(32),
	transferIndex INTEGER,
	blockNumber INTEGER,
	blockTime INTEGER,
	txID BLOB(32),
	txOrigin BLOB(20),
	fromAddress BLOB(20),
	toAddress BLOB(20),
	value BLOB
);

CREATE UNIQUE INDEX IF NOT EXISTS prim ON transfer(blockID, transferIndex);

CREATE INDEX IF NOT EXISTS blockNumberIndex ON transfer(blockNumber);
CREATE INDEX IF NOT EXISTS blockTimeIndex ON transfer(blockTime);
CREATE INDEX IF NOT EXISTS fromIndex ON transfer(fromAddress);
CREATE INDEX IF NOT EXISTS toIndex ON transfer(toAddress);
`
