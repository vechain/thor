package logdb

//LogSQL create a table for log
const LogSQL = `
create table if not exists log (
	blockID	char(66),
	blockNumber decimal(32,0),
	logIndex integer,
	txID char(66),
	txOrigin char(42),
	address char(42),
	data blob,
	topic0 char(66),
	topic1 char(66),
	topic2 char(66),
	topic3 char(66),
	topic4 char(66)
);

CREATE INDEX if not exists blockNumberIndex on log(blockNumber);
CREATE INDEX if not exists addressIndex on log(address);

CREATE INDEX if not exists topicIndex0 on log(topic0);
CREATE INDEX if not exists topicIndex1 on log(topic1);
CREATE INDEX if not exists topicIndex2 on log(topic2);
CREATE INDEX if not exists topicIndex3 on log(topic3);
CREATE INDEX if not exists topicIndex4 on log(topic4);
`
