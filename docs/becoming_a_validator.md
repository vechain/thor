# Becoming a Validator

Operating a validator requires having a synched mainnet node, a 25m VET endorsement and being added to the validator whitelist.

This document will outline the process of becoming an elected mainnet validator.

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

## Getting Whitelisted

Once the validator has synced the next step is to get added as one of the 101 whitelisted validators.

In order to get whitelisted the master key of a synched validator must be attached to an endorser address that maintains a minimum balance of 25m VET. 
The process of attaching the master key and the endorser involves an offchain identification and verification process under the current Proof of Authority (PoA) consensus mechanism. 
To begin the process of getting whitelisted reach out to our [support team](https://support.vechain.org/support/home) who will direct your to our dedicated onboarding team.

### Useful Commands

Use the following command to obtain the validator master key

```shell
bin/thor master-key
```
