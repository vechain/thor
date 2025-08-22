# Running a Discovery Node

Refer to the [Hardware Requirements](https://github.com/vechain/thor/tree/neil/docs-update?tab=readme-ov-file#hardware-requirements) and [Command Line Arguments](docs/command_line_arguments.md) for 
guidance on the hardware requirements and advanced node configuration.

To operate a discovery node the `disco` binary must be installed.

## Discovery Node Build

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

Build Thor and Disco:

```sh
make all
```

Run Disco:

```sh
bin/disco
```

Output:

The result will be an output similar to the below.

```shell
Running enode://e32e5960781ce0b43d8c2952eeea4b95e286b1bb5f8c1f0c9f09983ba7141d2fdd7dfbec798aefb30dcd8c3b9b7cda8e9a94396a0192bfa54ab285c2cec515ab@[::]:55555
```
