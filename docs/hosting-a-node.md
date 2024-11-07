## Hosting a Node

_**Please note**: The recommendations and information below are based on the main network as of 6th November 2024. The
requirements may change as the network evolves._

[VeChain Stats](https://vechainstats.com/charts/#thor-size) provides an up-to-date graphic of the network's current
state, including the disk space required for various node types.

### Table of Contents

- [System Requirements](#system-requirements)
    - [Authority Nodes](#authority-nodes)
    - [Full Archive Nodes](#full-archive-nodes)
- [Node Types](#node-types)
    - [Full Archive Node](#full-archive-node)
    - [Full Node](#full-node)
    - [Full Node without Logs](#full-node-without-logs)
- [Metrics](#metrics)

---

### Command Line Options

Please refer to [Command Line Options](./usage.md#command-line-options) in the usage documentation to see a list of all
available options.

---

### System Requirements

#### Authority Nodes

- Pruner enabled
- Skip logs

| Resource  | Minimum Specification | Recommended Specification |
|-----------|-----------------------|---------------------------|
| CPU       | 2 Core                | 4 Core                    |
| RAM       | 8 GB                  | 16 GB                     |
| Bandwidth | 10 Mbit               | 20 Mbit                   |
| Disk      | 200 GB NVMe SSD       | 300 GB NVMe SSD           |

### Full Archive Nodes

- Disabled pruner
- Enabled logs

| Resource  | Minimum Specification | Recommended Specification |
|-----------|-----------------------|---------------------------|
| CPU       | 2 Core                | 4 Core                    |
| RAM       | 16 GB                 | 32 GB                     |
| Bandwidth | 10 Mbit               | 20 Mbit                   |
| Disk      | 600 GB SSD            | 1 TB SSD                  |

---

### Node Types

#### Full Archive Node

A full archive node is a full node that stores all historical data of the blockchain, containing complete historical
data of all transactions and blocks, including forks and variations. Running a full archive node requires more resources
than running a regular full node, but it provides access to the complete history of the blockchain.

To run a full archive node, you need to set the `--disable-pruner` flag when starting the node. For example:

```shell
bin/thor --network main --disable-pruner
```

_As of 22nd April 2024, an archive node uses over **400 GB** of disk space._

#### Full Node

A full node is a node that stores the entire blockchain and validates transactions and blocks. Running a full node
requires fewer resources than running an archive node, but it still provides the same level of security and
decentralization.

Running a full node does not require any additional flags. For example:

```shell
bin/thor --network main
```

_As of 22nd April 2024, a full node uses **~200 GB** of disk space._

#### Full Node without Logs

- **Logs**: Logs are records of transfer and smart contract events stored in an SQLite database on the blockchain. When
  operating a node without logs, the /logs/event and /logs/transfer endpoints will be deactivated.
  These endpoints may experience CPU-intensive requests, causing performance issues. To address this, you can start a
  node without logs by using the --skip-logs flag. For example:

```shell
bin/thor --network main --skip-logs
```

_As of 22nd April 2024, a full node without logs uses **~100 GB** of disk space._

### Metrics

Telemetry plays a critical role in monitoring and managing blockchain nodes efficiently.
Below is an overview of how metrics is integrated and utilized within our node systems.

Metrics is enabled in nodes by default. It's possible to disable it by setting  `--enable-metrics=false`.
By default, a [prometheus](https://prometheus.io/docs/introduction/overview/) server is available at
`localhost:2112/metrics` with the metrics.

```shell
curl localhost:2112/metrics
```

Instrumentation is in a beta phase at this stage. You can read more about the metric
types [here](https://prometheus.io/docs/concepts/metric_types/).

### Admin

Admin is used to allow privileged actions to the node by the administrator. Currently it supports changing the logger's
verbosity at runtime.

Admin is not enabled in nodes by default. It's possible to enable it by setting  `--enable-admin`. Once enabled, an
Admin server is available at `localhost:2113/admin` with the following capabilities:

Retrieve the current log level via a GET request to /admin/loglevel.

```shell
curl http://localhost:2113/admin/loglevel
```

Change the log level via a POST request to /admin/loglevel.

```shell
curl -X POST -H "Content-Type: application/json" -d '{"level": "trace"}' http://localhost:2113/admin/loglevel
```
