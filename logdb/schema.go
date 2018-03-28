package logdb

// create a table for log
const logTableSchema = `
CREATE TABLE IF NOT EXISTS log (
	blockID	BLOB(32),
	logIndex INTEGER,
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

CREATE UNIQUE INDEX IF NOT EXISTS prim ON log(blockID, logIndex);

CREATE INDEX IF NOT EXISTS blockNumberIndex ON log(blockNumber);
CREATE INDEX IF NOT EXISTS blockTimeIndex ON log(blockTime);
CREATE INDEX IF NOT EXISTS addressIndex ON log(address);
CREATE INDEX IF NOT EXISTS topicIndex0 ON log(topic0);
CREATE INDEX IF NOT EXISTS topicIndex1 ON log(topic1);
CREATE INDEX IF NOT EXISTS topicIndex2 ON log(topic2);
CREATE INDEX IF NOT EXISTS topicIndex3 ON log(topic3);
CREATE INDEX IF NOT EXISTS topicIndex4 ON log(topic4);
`
