# Becoming a Validator

Validators are essential to the VeChainThor blockchain. They secure the network, participate in governance, and earn rewards for their contribution. By joining the leader group, validators play an active role in maintaining decentralization and ensuring consensus.

## Requirements

To become a validator the following criteria must be achieved:

- **Fully Synced Node:** Your node must be up-to-date with the latest VeChainThor block.
- **Stake Requirement:** A minimum of 25m VET must be deposited into the staking contract.
- **Leader Group Capacity:** The leader group is limited to 101 validators. Entry is on a first-in, first-out (FIFO) basis. If the leader group is at capacity, prospective validators are queued until space becomes available. 

## Validator Build

Refer to the [Hardware Requirements](https://github.com/vechain/thor/tree/master?tab=readme-ov-file#hardware-requirements), [Command Line Arguments](https://github.com/vechain/thor/blob/master/docs/command_line_arguments.md) and [Sync Time](https://github.com/vechain/thor/tree/master?tab=readme-ov-file#sync-time) for guidance on the hardware requirements, advanced node configuration and time required to sync the blockchain.

When operating a validator the recommended configuration is without logs.

### Prerequisites

Thor requires Go 1.19+ and a C compiler to build. Install them using your preferred package manager before continuing.

### Commands

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
bin/thor --network main --skip-logs
```

Thor will begin syncing the mainnet and can be accessed at [http://localhost:8669/](http://localhost:8669/).

## Joining the Leader Group

The leader group consists of 101 validators that are elected to participate in VeChainThor's consensus. To join the leader group a prospective validator must have a fully synced node and stake a minimum of 25m VET.

Validators join the leader group on a first-in, first out (FIFO) basis, provided there is available capacity. Once a validator enters the leader group, their stake lock period begins and they are eligible to earn rewards.

A validator must provide the following parameters as part of the staking action:

- **Caller:** The endorser public address, the account depositing the VET into the staking contract.
- **Validator:** The master key public address, the account generated when starting a node.
- **Period:** The stake lock period, the time period, in blocks, that the validator commits to hard lock their VET into the staking contract. The options are 60480 (7 days), 129600 (15 days) or 259200 (30 days).
- **Value:** The quantity of VET that will be deposited into the staking contract from the caller / endorser pubilc address.

Reach out to our [support team](https://support.vechain.org/support/home) if there are any questions or queries in relation to joining the network as a validator.

### Useful Commands

Use the following command to obtain the validator master key public address

```shell
bin/thor master-key
```
