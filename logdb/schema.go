package logdb

// create a table for log
const logTableSchema = `
create table if not exists log (
	blockID	blob(32),
	blockNumber decimal(32,0),
	logIndex integer,
	txID blob(32),
	txOrigin blob(20),
	address blob(20),
	data blob,
	topic0 blob(32),
	topic1 blob(32),
	topic2 blob(32),
	topic3 blob(32),
	topic4 blob(32)
);

CREATE INDEX if not exists blockNumberIndex on log(blockNumber);
CREATE INDEX if not exists addressIndex on log(address);

CREATE INDEX if not exists topicIndex0 on log(topic0);
CREATE INDEX if not exists topicIndex1 on log(topic1);
CREATE INDEX if not exists topicIndex2 on log(topic2);
CREATE INDEX if not exists topicIndex3 on log(topic3);
CREATE INDEX if not exists topicIndex4 on log(topic4);
`
