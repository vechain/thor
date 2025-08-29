# Command Line Arguments

Thor, Thor Solo and Discovery nodes all have advanced configurations available to help customise the Thor Client to the users desires. Below you will see a list of all of the available command line options.

To show usages of all command line options:

```shell
bin/thor -h
```

## Thor Commands

| Commands                         | Description                                                                                                                    |
|----------------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `--network`                      | The network to join (main\|test) or path to the genesis file                                                                   |
| `--data-dir`                     | Directory for blockchain databases                                                                                             |
| `--beneficiary`                  | Address for block rewards                                                                                                      |
| `--api-addr`                     | API service listening address (default: "localhost:8669")                                                                      |
| `--api-cors`                     | Comma-separated list of domains from which to accept cross-origin requests to API                                              |
| `--api-timeout`                  | API request timeout value in milliseconds (default: 10000)                                                                     |
| `--api-call-gas-limit`           | Limit contract call gas (default: 50000000)                                                                                    |
| `--api-backtrace-limit`          | Limit the distance between 'position' and best block for subscriptions and fees APIs (default: 1000)                           |
| `--api-allow-custom-tracer`      | Allow custom JS tracer to be used for the tracer API                                                                           |
| `--api-allowed-tracers`          | Comma-separated list of allowed tracers (default: "none")                                                                      |
| `--enable-api-logs`              | Enables API requests logging                                                                                                   |
| `--api-logs-limit`               | Limit the number of logs returned by /logs API (default: 1000)                                                                 |
| `--api-priority-fees-percentage` | Percentage of the block base fee for priority fees calculation (default: 5)                                                    |
| `--verbosity`                    | Log verbosity (0-9) (default: 3)                                                                                               |
| `--max-peers`                    | Maximum number of P2P network peers (P2P network disabled if set to 0) (default: 25)                                           |
| `--p2p-port`                     | P2P network listening port (default: 11235)                                                                                    |
| `--nat`                          | Port mapping mechanism (any\|none\|upnp\|pmp\|extip:<IP>) (default: "any")                                                     |
| `--bootnode`                     | Comma separated list of bootnode IDs                                                                                           |
| `--target-gas-limit`             | Target block gas limit (adaptive if set to 0) (default: 0)                                                                     |
| `--pprof`                        | Turn on go-pprof                                                                                                               |
| `--skip-logs`                    | Skip writing event\|transfer logs (/logs API will be disabled)                                                                 |
| `--cache`                        | Megabytes of RAM allocated to trie nodes cache (default: 4096)                                                                 |
| `--disable-pruner`               | Disable state pruner to keep all history                                                                                       |
| `--enable-metrics`               | Enables the metrics server                                                                                                     |
| `--metrics-addr`                 | Metrics service listening address                                                                                              |
| `--enable-admin`                 | Enables the admin server                                                                                                       |
| `--admin-addr`                   | Admin service listening address                                                                                                |
| `--txpool-limit-per-account`     | Transaction pool size limit per account                                                                                        |
| `--min-effective-priority-fee`   | Sets a minimum effective priority fee for transactions to be included in the block proposed by the block proposer (default: 0) |
| `--help, -h`                     | Show help                                                                                                                      |
| `--version, -v`                  | Print the version                                                                                                              |

## Thor Solo Commands

| Commands                     | Description                                        |
|------------------------------|----------------------------------------------------|
| `--genesis`                  | Path to genesis file(default: builtin devnet)      |
| `--on-demand`                | Create new block when there is pending transaction |
| `--block-interval`           | Choose a block interval in seconds (default 10s)   |
| `--persist`                  | Save blockchain data to disk(default to memory)    |
| `--gas-limit`                | Gas limit for each block                           |
| `--txpool-limit`             | Transaction pool size limit                        |


## Discovery Node Commands

To show all command line options:

```shell
bin/disco -h
```

| Flag            | Description                                                                             |
|-----------------|-----------------------------------------------------------------------------------------|
| `--addr`        | The to listen on (default: ":55555").                                                   |
| `--keyfile`     | The path to the private key file of the discovery node.                                 |
| `--keyhex`      | The hex-encoded private key of the discovery node.                                      |
| `--nat`         | The port mapping mechanism (any \| none \| upnp \| pmp \| extip:<IP>) (default: "none") |
| `--netrestrict` | Restrict network communication to the given IP networks (CIDR masks)                    |
| `--verbosity`   | The log level for the discovery node (0-9) (default: 2).                                |
| `--help`        | Show the help message for the discovery node.                                           |
| `--version`     | Show the version of the discovery node.                                                 |

## Useful Notes

- Add the `--api-addr 0.0.0.0:8669` flag if you want other containers and/or hosts to have access to the
RESTful API. `Thor` binds to `localhost` by default and it will not accept requests outside the container itself without
the flag._

- Release [v2.0.4](https://github.com/vechain/thor/releases/tag/v2.0.4) changed the default user from `root` (UID: 0)
to `thor` (UID: 1000). Ensure that UID 1000 has `rwx` permissions on the data directory of the docker host. You can do
that with ACL `sudo setfacl -R -m u:1000:rwx {path-to-your-data-directory}`, or update ownership
with `sudo chown -R 1000:1000 {path-to-your-data-directory}`.

### Sub-commands

#### Thor Solo

`thor solo` is a sub-command for running a single node in a standalone mode. It is useful for testing and development. See [Thor Solo](https://github.com/vechain/thor/blob/master/docs/thor_solo.md) for more information.

```shell
# create new block when there is pending transaction
bin/thor solo --on-demand

# save blockchain data to disk(default to memory)
bin/thor solo --persist

# two options can work together
bin/thor solo --persist --on-demand
```

#### Master Key

`thor master-key` is a sub-command for managing the node's master key.

```shell
# print the master address
bin/thor master-key

# export master key to keystore
bin/thor master-key --export > keystore.json


# import master key from keystore
cat keystore.json | bin/thor master-key --import
```

#### Metrics

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

#### Admin

Admin is used to allow privileged actions to the node by the administrator.

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

#### Health

Retrieve the node health infomation via a GET request to /admin/health.

```shell
curl http://localhost:2113/admin/health
```

Response Example

```json
{
    "healthy": true,
    "bestBlockTime": "2025-07-01T06:50:00Z",
    "peerCount": 5,
    "isNetworkProgressing": true
}
```

|           Key         |           Type        |         Description       |
|-----------------------|-----------------------|---------------------------|
| healthy               | boolean               | If the peerCount <= `min_peers_count` and `isNetworkProgressing` is True, it will return True.(default `min_peers_count` is 2)|
| bestBlockTime         | string                | The best block time of the node.                     |
| peerCount             | number                | The number of peers connected to the node.                  |
| isNetworkProgressing  | boolean               | If the node has not completed the block sync, it will return False  |

- **Note**: if the `healthy` is False, the response status code is 503
