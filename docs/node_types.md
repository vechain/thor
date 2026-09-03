# Node Types

## Full Node

A full node is a node that stores the entire blockchain and validates transactions and blocks. Running a full node
requires fewer resources than running an archive node, but it still provides the same level of security and
decentralization.

Running a full node does not require any additional flags when starting the node.

```shell
bin/thor --network main
```

_As of 22nd April 2024, a full node uses **~200 GB** of disk space._

## Full Node without Logs

This is the recommended validator build in the VeChainThor network.

- **Logs**: Logs are records of transfers and smart contract events stored in an SQLite database on the blockchain. When
  operating a node without logs, the /logs/event and /logs/transfer endpoints will be deactivated.
  These endpoints may experience CPU-intensive requests, causing performance issues. To address this, you can start a
  node without logs by using the `--skip-logs` flag. For example:

Running a full node, without logs requires using the `--skip-logs` command when starting the node.

```shell
bin/thor --network main --skip-logs
```

_As of 22nd April 2024, a full node without logs uses **~100 GB** of disk space._

## Full Archive Node

A full archive node is a full node that stores all historical data of the blockchain, containing complete historical
data of all transactions and blocks, including forks and variations. Running a full archive node requires more resources
than running a regular full node, but it provides access to the complete history of the blockchain.

To run a full archive node, requires using the `--disable-pruner` command when starting the node.

```shell
bin/thor --network main --disable-pruner
```

_As of 22nd April 2024, an archive node uses over **400 GB** of disk space._
