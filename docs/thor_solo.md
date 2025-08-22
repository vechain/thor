# Thor Solo

## Overview and Purpose

A Thor Solo Node is a standalone local and independent instance of the VeChainThor blockchain client. It operates in isolation, performing all validation and block‐creation tasks on its own.

Thor Solo Nodes are purpose-built for development and testing. It offers several benefits:

- **Lightweight and quick setup:** They don't need the entire blockchain history, making initialization fast and simple.
- **Controlled sandbox environment:** Ideal for simulating specific conditions or testing smart contracts without dependency on network peers.
- **Predictable behavior:** Enables deterministic testing of contracts, block generation and API responses.

However, Thor Solo Nodes do come with limitations:

- **Lacks real-world conditions:** No network latency, congestion, or peer diversity.
- **Under-exposed security surface:** Doesn't replicate real-world attack vectors or consensus behavior.
- **Not indicative of scalability / performance:** Doesn't simulate peer-to-peer load distribution or stress scenarios.

## Build a Thor Solo Node

### Build and Run from Source

#### Prerequisites

Thor requires Go 1.19+ and a C compiler to build. Install them using your preferred package manager before continuing.

#### Commands

Clone the Thor repo:

```sh
git clone https://github.com/vechain/thor.git
```

Enter the Thor directory:

```sh
cd thor
```

Checkout the latest stable release:

```sh
git checkout $(git describe --tags `git rev-list --tags --max-count=1`)
```

Build Thor:

```sh
make
```

Run Thor Solo:

```sh
bin/thor solo --api-addr 0.0.0.0:8669
```

Thor Solo will launch and can be accessed at [http://localhost:8669/](http://localhost:8669/).

### Build and Run with Docker

#### Docker CLI

```sh
docker run -p 127.0.0.1:8669:8669 vechain/thor:latest solo --api-cors '*' --api-addr 0.0.0.0:8669
```

Thor Solo will launch and can be accessed at [http://localhost:8669/](http://localhost:8669/).

This will start a contanerized Thor Solo Node with:

- Use the latest Thor Solo release
- Localhost access on port 8669
- Unrestricted cross-origin requests
- Remote connection capability

## Advanced Configuration

The Thor Solo node can be configured with additional commands to assist in speeding up testing and validating smart contracts in a local and independent instance of a VeChainThor blockchain client.

### Useful Thor Commands

The following are a subset of the key Thor commands that can be used when operating a Thor Solo Node. A full list of Thor commands are available [here](https://github.com/vechain/thor/blob/neil/docs-update/docs/command_line_arguments.md).

| Commands                         | Description                                                                                                                    |
|----------------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `--api-addr`                     | API service listening address (default: "localhost:8669")                                                                      |
| `--api-cors`                     | Comma-separated list of domains from which to accept cross-origin requests to API                                              |
| `--api-call-gas-limit`           | Limit contract call gas (default: 50000000)                                                                                    |
| `--verbosity`                    | Log verbosity (0-9) (default: 3)                                                                                               |

### Thor Solo Specific

The following commands are specific to Thor Solo.

| Commands                     | Description                                        |
|------------------------------|----------------------------------------------------|
| `--genesis`                  | Path to genesis file(default: builtin devnet)      |
| `--on-demand`                | Create new block when there is pending transaction |
| `--block-interval`           | Choose a block interval in seconds (default 10s)   |
| `--persist`                  | Save blockchain data to disk(default to memory)    |
| `--gas-limit`                | Gas limit for each block                           |
| `--txpool-limit`             | Transaction pool size limit                        |

An example genesis config file can be found [here](https://raw.githubusercontent.com/vechain/thor/master/genesis/example.json).

