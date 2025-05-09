## VechainThor Usage

___

### Table of Contents

- [Running from source](#running-from-source)
- [Running a discovery node](#running-a-discovery-node)
- [Running with Docker](#running-with-docker)
- [Docker Compose](#docker-compose)
- [Sub-commands](#sub-commands)
    - [Thor Solo](#thor-solo)
    - [Master Key](#master-key)
- [Command line options](#command-line-options)
    - [Thor Solo Flags](#thor-solo-flags)
    - [Discovery Node](#discovery-node-flags)
- [Open API Documentation](#open-api-documentation)

___

### Running from source

- To install the `thor` binary, follow the instructions in the [build](https://github.com/vechain/thor/blob/master/docs/build.md) guide.

Connect to vechain's mainnet:

```shell
bin/thor --network main
```

Connect to vechain's testnet:

```shell
bin/thor --network test
```

Start a custom network:

```shell
bin/thor --network <custom-net-genesis.json>
```

An example genesis config file can be found
at [genesis/example.json](https://raw.githubusercontent.com/vechain/thor/master/genesis/example.json).

___

### Running a discovery node

- To install the `disco` binary, follow the instructions in the [build](https://github.com/vechain/thor/blob/master/docs/build.md) guide.

Start a discovery node:

```shell
disco
```

Output:

```shell
Running enode://e32e5960781ce0b43d8c2952eeea4b95e286b1bb5f8c1f0c9f09983ba7141d2fdd7dfbec798aefb30dcd8c3b9b7cda8e9a94396a0192bfa54ab285c2cec515ab@[::]:55555
```

___

### Running with Docker

Docker is one quick way for running a vechain node:

```shell
docker run -d\
  -v {path-to-your-data-directory}/.org.vechain.thor:/home/thor/.org.vechain.thor\
  -p 127.0.0.1:8669:8669 -p 11235:11235 -p 11235:11235/udp\
  --name thor-node vechain/thor --network test
```


Notes: 

- Add the `--api-addr 0.0.0.0:8669` flag if you want other containers and/or hosts to have access to the
RESTful API. `Thor` binds to `localhost` by default and it will not accept requests outside the container itself without
the flag._

- Release [v2.0.4](https://github.com/vechain/thor/releases/tag/v2.0.4) changed the default user from `root` (UID: 0)
to `thor` (UID: 1000). Ensure that UID 1000 has `rwx` permissions on the data directory of the docker host. You can do
that with ACL `sudo setfacl -R -m u:1000:rwx {path-to-your-data-directory}`, or update ownership
with `sudo chown -R 1000:1000 {path-to-your-data-directory}`.

___

### Docker Compose

A `docker-compose.yml` file is provided for convenience. It will create a container with the same configuration as the
command above.

```yaml
version: '3'

services:
  thor-node:
    image: vechain/thor
    container_name: thor-node
    command: --network test --api-addr 0.0.0.0:8669
    volumes:
      - thor-data:/home/thor
    ports:
      - "8669:8669"
      - "11235:11235"
      - "11235:11235/udp"

volumes:
  thor-data:
```

___

### Sub-commands

#### Thor Solo

`thor solo` is a sub-command for running a single node in a standalone mode. It is useful for testing and development.

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

___

### Command line options

To show usages of all command line options:

```shell
bin/thor -h
```

| Flag                                     | Description                                                                                          |
|------------------------------------------|------------------------------------------------------------------------------------------------------|
| `--network`                              | The network to join (main\|test) or path to the genesis file                                         |
| `--data-dir`                             | Directory for blockchain databases                                                                   |
| `--beneficiary`                          | Address for block rewards                                                                            |
| `--api-addr`                             | API service listening address (default: "localhost:8669")                                            |
| `--api-cors`                             | Comma-separated list of domains from which to accept cross-origin requests to API                    |
| `--api-timeout`                          | API request timeout value in milliseconds (default: 10000)                                           |
| `--api-call-gas-limit`                   | Limit contract call gas (default: 50000000)                                                          |
| `--api-backtrace-limit`                  | Limit the distance between 'position' and best block for subscriptions and fees APIs (default: 1000) |
| `--api-allow-custom-tracer`              | Allow custom JS tracer to be used for the tracer API                                                 |
| `--api-allowed-tracers`                  | Comma-separated list of allowed tracers (default: "none")                                            |
| `--enable-api-logs`                      | Enables API requests logging                                                                         |
| `--api-logs-limit`                       | Limit the number of logs returned by /logs API (default: 1000)                                       |
| `--api-priority-fees-backtrace-limit`    | Limit the distance with the best block for priority fees calculation (default: 20)                   |
| `--api-priority-fees-percentile`         | Percentile for priority fees calculation (default: 60)                                               |
| `--api-priority-fees-sample-tx-per-block`| Number of transactions to sample per block for priority fees calculation (default: 3)                |
| `--verbosity`                            | Log verbosity (0-9) (default: 3)                                                                     |
| `--max-peers`                            | Maximum number of P2P network peers (P2P network disabled if set to 0) (default: 25)                 |
| `--p2p-port`                             | P2P network listening port (default: 11235)                                                          |
| `--nat`                                  | Port mapping mechanism (any\|none\|upnp\|pmp\|extip:<IP>) (default: "any")                           |
| `--bootnode`                             | Comma separated list of bootnode IDs                                                                 |
| `--target-gas-limit`                     | Target block gas limit (adaptive if set to 0) (default: 0)                                           |
| `--pprof`                                | Turn on go-pprof                                                                                     |
| `--skip-logs`                            | Skip writing event\|transfer logs (/logs API will be disabled)                                       |
| `--cache`                                | Megabytes of RAM allocated to trie nodes cache (default: 4096)                                       |
| `--disable-pruner`                       | Disable state pruner to keep all history                                                             |
| `--enable-metrics`                       | Enables the metrics server                                                                           |
| `--metrics-addr`                         | Metrics service listening address                                                                    |
| `--enable-admin`                         | Enables the admin server                                                                             |
| `--admin-addr`                           | Admin service listening address                                                                      |
| `--txpool-limit-per-account`             | Transaction pool size limit per account                                                              |
| `--help, -h`                             | Show help                                                                                            |
| `--version, -v`                          | Print the version                                                                                    |

#### Thor Solo Flags

| Flag                         | Description                                        |
|------------------------------|----------------------------------------------------|
| `--genesis`                  | Path to genesis file(default: builtin devnet)      |
| `--on-demand`                | Create new block when there is pending transaction |
| `--block-interval`           | Choose a block interval in seconds (default 10s)   |
| `--persist`                  | Save blockchain data to disk(default to memory)    |
| `--gas-limit`                | Gas limit for each block                           |
| `--txpool-limit`             | Transaction pool size limit                        |


#### Discovery Node Flags

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

___

### Open API Documentation

Once `thor` has started, the online *OpenAPI* documentation can be accessed in your browser.
e.g. [http://localhost:8669/](http://localhost:8669) by default.

[![Thorest](https://raw.githubusercontent.com/vechain/thor/master/thorest.png)](http://localhost:8669/)
