# Vechain Thor

A general purpose blockchain highly compatible with Ethereum's ecosystem.

This is the first implementation written in golang.

[![Go](https://img.shields.io/badge/golang-%3E%3D1.19-orange.svg)](https://golang.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/vechain/thor)](https://goreportcard.com/report/github.com/vechain/thor)
![GitHub Action Status](https://github.com/vechain/thor/actions/workflows/test.yaml/badge.svg)
[![License](https://img.shields.io/badge/License-LGPL%20v3-blue.svg)](https://github.com/vechain/thor/blob/master/LICENSE)
&nbsp;&nbsp; [![TG](https://img.shields.io/badge/chat-on%20telegram-blue)](https://t.me/VechainDevCommunity)

## Table of contents

* [Installation](#installation)
  * [Requirements](#requirements)
  * [Clone the repo](#clone-the-repo)
  * [Building](#building)
* [Running Thor](#running-thor)
  * [Sub-commands](#sub-commands)
* [Docker](#docker)
* [Explorers](#explorers)
* [Faucet](#testnet-faucet)
* [RESTful API](#api)
* [Acknowledgement](#acknowledgement)
* [Contributing](#contributing)

## Installation

### Requirements

Thor requires `Go` 1.19+ and `C` compiler to build. To install `Go`, follow this [link](https://golang.org/doc/install).

### Clone the repo

Clone the Thor repo:

```shell
git clone https://github.com/vechain/thor.git
cd thor
```

To see a list of all available commands, run `make help`

### Building

To build the main app `thor`, just run

```shell
make
```

or build the full suite:

```shell
make all
```

If no errors are reported, all built executable binaries will appear in folder *bin*.

## Running Thor

Connect to Vechain's mainnet:

```shell
bin/thor --network main
```

Connect to Vechain's testnet:

```shell
bin/thor --network test
```

or startup a custom network

```shell
bin/thor --network <custom-net-genesis.json>
```

An example genesis config file can be found at [genesis/example.json](https://raw.githubusercontent.com/vechain/thor/master/genesis/example.json).

To show usages of all command line options:

```shell
bin/thor -h
```


| Flag                        | Description                                                                                                |
|-----------------------------|------------------------------------------------------------------------------------------------------------|
| `--network`                 | The network to join (main\|test) or path to the genesis file                                               |
| `--data-dir`                | Directory for blockchain databases (default: "/Users/darren/Library/Application Support/org.vechain.thor") |
| `--cache`                   | Megabytes of RAM allocated to trie nodes cache (default: 4096)                                             |
| `--beneficiary`             | Address for block rewards                                                                                  |
| `--target-gas-limit`        | Target block gas limit (adaptive if set to 0) (default: 0)                                                 |
| `--api-addr`                | API service listening address (default: "localhost:8669")                                                  |
| `--api-cors`                | Comma-separated list of domains from which to accept cross-origin requests to API                          |
| `--api-timeout`             | API request timeout value in milliseconds (default: 10000)                                                 |
| `--api-call-gas-limit`      | Limit contract call gas (default: 50000000)                                                                |
| `--api-backtrace-limit`     | Limit the distance between 'position' and best block for subscriptions APIs (default: 1000)                |
| `--api-allow-custom-tracer` | Allow custom JS tracer to be used for the tracer API                                                       |
| `--verbosity`               | Log verbosity (0-9) (default: 3)                                                                           |
| `--max-peers`               | Maximum number of P2P network peers (P2P network disabled if set to 0) (default: 25)                       |
| `--p2p-port`                | P2P network listening port (default: 11235)                                                                |
| `--nat`                     | Port mapping mechanism (any\|none\|upnp\|pmp\|extip:<IP>) (default: "any")                                 |
| `--bootnode`                | Comma-separated list of bootnode IDs                                                                       |
| `--skip-logs`               | Skip writing event\|transfer logs (/logs API will be disabled)                                             |
| `--pprof`                   | Turn on go-pprof                                                                                           |
| `--disable-pruner`          | Disable state pruner to keep all history                                                                   |
| `--help, -h`                | Show help                                                                                                  |
| `--version, -v`             | Print the version                                                                                          |
| `--connect-only-nodes`      | Only connect to certain nodes. Set the enode for the nodes.                                                |

### Sub-commands

* `solo`                client runs in solo mode for test & dev

```shell
# create new block when there is pending transaction
bin/thor solo --on-demand

# save blockchain data to disk(default to memory)
bin/thor solo --persist

# two options can work together
bin/thor solo --persist --on-demand
```

* `master-key`          master key management

```shell
# print the master address
bin/thor master-key

# export master key to keystore
bin/thor master-key --export > keystore.json


# import master key from keystore
cat keystore.json | bin/thor master-key --import
```

## Docker

Docker is one quick way for running a vechain node:

```shell
docker run -d\
  -v {path-to-your-data-directory}/.org.vechain.thor:/home/thor/.org.vechain.thor\
  -p 127.0.0.1:8669:8669 -p 11235:11235 -p 11235:11235/udp\
  --name thor-node vechain/thor --network test
```

_Do not forget to add the `--api-addr 0.0.0.0:8669` flag if you want other containers and/or hosts to have access to the RESTful API. `Thor` binds to `localhost` by default and it will not accept requests outside the container itself without the flag._

Release [v2.0.4](https://github.com/vechain/thor/releases/tag/v2.0.4) changed the default user from `root` (UID: 0) to `thor` (UID: 1000). Ensure that UID 1000 has `rwx` permissions on the data directory of the docker host. You can do that with ACL `sudo setfacl -R -m u:1000:rwx {path-to-your-data-directory}`, or update ownership with `sudo chown -R 1000:1000 {path-to-your-data-directory}`.


### Docker Compose

A `docker-compose.yml` file is provided for convenience. It will create a container with the same configuration as the command above.

```yaml
version: '3.8.'

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

## Explorers

* [Vechain Explorer (Official)](https://explore.vechain.org)
* [VechainStats](https://vechainstats.com/)
* [Insight](https://insight.vecha.in/)

## Testnet faucet

* [faucet.vecha.in](https://faucet.vecha.in) by *Vechain Foundation*

## API

Once `thor` has started, the online *OpenAPI* documentation can be accessed in your browser. e.g. [http://localhost:8669/](http://localhost:8669) by default.

[![Thorest](https://raw.githubusercontent.com/vechain/thor/master/thorest.png)](http://localhost:8669/)

## Acknowledgement

A special shout out to following projects:

* [Ethereum](https://github.com/ethereum)
* [Swagger](https://github.com/swagger-api)
* [Stoplight Elements](https://github.com/stoplightio/elements)

## Contributing

- Please refer to [CONTRIBUTING.md](https://github.com/vechain/thor/blob/master/.github/CONTRIBUTING.md) on how to contribute to this project.

## License

Vechain Thor is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.html), also included
in *LICENSE* file in repository.
