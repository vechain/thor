## Hosting a Node

_**Please note**: The recommendations and information below are based on the main network as of 22nd April 2024. The
requirements may change as the network evolves._

### Table of Contents

- [System Requirements](#system-requirements)
  - [Authority Nodes](#authority-nodes)
  - [Public Nodes](#public-nodes)
- [Important Considerations](#important-considerations)
  - [Archive Node](#archive-node)
  - [Full Node](#full-node)
  - [Full Node without Logs](#full-node-without-logs)
- [Telemetry](#telemetry)

___

### Command Line Options

Please refer to [Command Line Options](./usage.md#command-line-options) in the usage documentation to see a list of all available options.
___

### System Requirements

#### Authority Master Nodes

Below spec is the node configured in full node without logs.

| Resource  | Minimum Specification | Recommended Specification |
|-----------|-----------------------|---------------------------|
| CPU       | 2 Core                | 4 Core                    |
| RAM       | 8 GB                  | 16 GB                     |
| Bandwidth | 10 Mbit               | 20 Mbit                   |
| Disk      | 300 GB NVMe SSD            | 500 GB NVMe SSD        |

#### Public Nodes

**Note**: For public nodes, it is essential to configure them with a robust and secure setup, including protection
against DDoS attacks and intrusion detection systems (IDS).

Below spec is the node configured in full archive node.

| Resource  | Minimum Specification | Recommended Specification |
|-----------|-----------------------|---------------------------|
| CPU       | 8 Core                | 16 Core                   |
| RAM       | 16 GB                 | 64 GB                     |
| Bandwidth | 10 Mbit               | 20 Mbit                   |
| Disk      | 600 GB SSD            | 1 TB SSD                  |

___

### Important Considerations

#### Full Archive Node

A full archive node is a full node that stores all historical data of the blockchain, containing complete historical data of
all transactions and blocks, including forks and variations. Running a full archive node requires more resources than
running a regular full node, but it provides access to the complete history of the blockchain.

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

**Note**: Logs pertain to the transfer and smart contract events recorded on the blockchain, meticulously stored
within an SQLite database for streamlined querying purposes. The `/logs/event` and `/logs/transfer` endpoints will be
deactivated when operating a node without logs.

To run a full node without logs, you need to set the `--skip-logs` flag when starting the node. For example:

```shell
bin/thor --network main --skip-logs
```

_As of 22nd April 2024, a full node without logs uses **~100 GB** of disk space._


### Telemetry

Telemetry plays a critical role in monitoring and managing blockchain nodes efficiently. 
Below is an overview of how telemetry is integrated and utilized within our node systems.

Telemetry is enabled in nodes by default. It's possible to disable it by setting  `--telemetry-enabled=false`.
By default, a [prometheus](https://prometheus.io/docs/introduction/overview/) server is available at `localhost:2112/metrics` with the metrics.

```shell
curl localhost:2112/metrics
```

Instrumentation is in a beta phase at this stage. You can read more about the metric types [here](https://prometheus.io/docs/concepts/metric_types/).