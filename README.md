
<p align="center">
  <a href="https://www.vechain.org/vechainthor/">
    <picture style="padding: 80px;">
        <img src="https://raw.githubusercontent.com/vechain/thor/refs/heads/neil/docs-update/docs/assets/banner.png" style="padding: 20px;">
    </picture>
  </a>
</p>

---

<p align="center">
    <a href="https://golang.org"><img src="https://img.shields.io/github/go-mod/go-version/vechain/thor"/></a>
    <a href="https://github.com/vechain/thor/blob/master/LICENSE"><img src="https://img.shields.io/badge/License-LGPL%20v3-blue.svg"/></a>
    <img src="https://img.shields.io/github/commits-since/vechain/thor/latest" />
    <a href="https://hub.docker.com/r/vechain/thor"><img src="https://badgen.net/docker/pulls/vechain/thor?icon=docker&label=pulls"/></a>
</p>

<p align="center">
    <a href="https://goreportcard.com/report/github.com/vechain/thor"><img src="https://goreportcard.com/badge/github.com/vechain/thor"/></a>
    <img src="https://github.com/vechain/thor/actions/workflows/on-master-commit.yaml/badge.svg"/>
    <img src="https://github.com/vechain/thor/actions/workflows/on-release.yaml/badge.svg"/>
    <a href="https://codecov.io/gh/vechain/thor"><img src="https://codecov.io/gh/vechain/thor/graph/badge.svg?token=NniVYY7IAD"/></a>
</p>

<p align="center">
    <a href="https://discord.gg/vechain"><img src="https://img.shields.io/badge/Discord-5865F2?style=for-the-badge&logo=discord&logoColor=white"/></a>
    <a href="https://t.me/vechainandfriends"><img src="https://img.shields.io/badge/Telegram-2CA5E0?style=for-the-badge&logo=telegram&logoColor=white"/></a>
    <a href="https://www.reddit.com/r/Vechain"><img src="https://img.shields.io/badge/Reddit-FF4500?style=for-the-badge&logo=reddit&logoColor=white"/></a>
</p>

---

# Thor: The VeChainThor Client

Thor is the official Golang client for VeChainThor, the public blockchain powering the VeChain ecosystem.  
VeChainThor is designed for real-world adoption, enabling scalable, low-cost, and sustainable applications.

> VeChainThor is currently up-to-date with the EVM's `shanghai` hardfork.  
> Set [`evmVersion`](https://docs.soliditylang.org/en/latest/using-the-compiler.html#setting-the-evm-version-to-target) to `shanghai` if you are using Solidity compiler version `0.8.25` or above.

## Hardware Requirements

| Resource  | Validator       | Public Full Node |
|-----------|-----------------|------------------|
| CPU       | 4 Core          | 8 Core           |
| RAM       | 16 GB           | 64 GB            |
| Bandwidth | 20 Mbit         | 20 Mbit          |
| Disk      | 500 GB NVMe SSD | 1 TB SSD         |

Minimum of 45,000 IOPS required for an approximate 30 hour sync time.

## Sync Time

Sync time from genesis to the latest mainnet block depends on hardware, configuration, and bandwidth.

### Validator

| Build                                         | Sync Time            | AWS SKU                                                      |
|-----------------------------------------------|----------------------|--------------------------------------------------------------|
| 4 CPU, 32 GB, 10 Mbit, 937 NVMe SSD, 10K IOPS | 54 Hours, 08 Minutes | [I4g.xlarge](https://aws.amazon.com/ec2/instance-types/i4g/) |
| 2 CPU, 16 GB, 10 Mbit, 468 NVMe SSD, 35K IOPS | 38 Hours, 41 Minutes | [I8g.large](https://aws.amazon.com/ec2/instance-types/i8g/)  |
| 4 CPU, 32 GB, 10 Mbit, 937 NVMe SSD, 45K IOPS | 30 Hours, 00 Minutes | [I8g.xlarge](https://aws.amazon.com/ec2/instance-types/i8g/) |

### Public Full Node

| Build                                          | Sync Time            | AWS SKU                                                       |
|------------------------------------------------|----------------------|---------------------------------------------------------------|
| 8 CPU, 64 GB, 12 Mbit, 1875 NVMe SSD, 15K IOPS | 59 Hours, 04 Minutes | [I4g.2xlarge](https://aws.amazon.com/ec2/instance-types/i4g)  |
| 8 CPU, 64 GB, 12 Mbit, 1875 NVMe SSD, 42K IOPS | 32 Hours, 17 Minutes | [I8g.2xlarge](https://aws.amazon.com/ec2/instance-types/i8g/) |

## Installation

Use either the source or Docker instructions below to start a mainnet validator or public full node.

Becoming a validator requires meeting endorsement criteria and being voted in. See [Becoming a Validator](#) for more details.

The thor build below uses minimal configuration options. For more configuration options, see [Command Line Arguments](#).

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

Run Thor:

```sh
bin/thor --network main
```

Thor will begin syncing the mainnet and can be accessed at [http://localhost:8669/](http://localhost:8669/).

### Build and Run with Docker

#### Docker CLI

```sh
docker run -d\
  -v {path-to-your-data-directory}/.org.vechain.thor:/home/thor/.org.vechain.thor\
  --api-addr 0.0.0.0:8669 -p 127.0.0.1:8669:8669 -p 11235:11235 -p 11235:11235/udp\
  --name thor-node vechain/thor --network main
```

Thor will begin syncing the mainnet and can be accessed at [http://localhost:8669/](http://localhost:8669/).

#### Docker Compose

Use the provided `docker-compose.yml` to launch a node with the same configuration:

```yaml
version: '3'

services:
  thor-node:
    image: vechain/thor
    container_name: thor-node
    command: --network main --api-addr 0.0.0.0:8669
    volumes:
      - thor-data:/home/thor
    ports:
      - "8669:8669"
      - "11235:11235"
      - "11235:11235/udp"

volumes:
  thor-data:
```

Thor will begin syncing the mainnet and can be accessed at [http://localhost:8669/](http://localhost:8669/).

## Documentation

- [Becoming a Validator](#)
- [Command Line Arguments](#)
- [Node Types](#)
- [Thor Solo](#)
- [Running a Discovery Node](#)

## Contributing

Contributions are welcome and appreciated!  
Please review our [Contribution Guidelines](https://github.com/vechain/thor/blob/master/docs/CONTRIBUTING.md) before submitting a PR.

## Security

If you discover a security vulnerability, please report it according to our  
[Security Policy](https://github.com/vechain/thor/blob/master/docs/SECURITY.md).

## Acknowledgements

Special thanks to the following projects:

- [Ethereum](https://github.com/ethereum)
- [Go-Ethereum](https://github.com/ethereum/go-ethereum)
- [Swagger](https://github.com/swagger-api)
- [Stoplight Elements](https://github.com/stoplightio/elements)

## License

VeChainThor is licensed under the  
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.html),  
also available in the `LICENSE` file in this repository.
