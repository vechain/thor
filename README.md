## VeChain Thor

Thor is VeChain's new generation blockchain project.  It's the official implementation written in golang.

## Installation

### Requirements

Thor requires `Go` 1.10+ and `C` compiler to build. To install `Go`, follow this [link](https://golang.org/doc/install). 

In addition, [dep](https://github.com/golang/dep) is required to manage dependencies. 

### Getting the source

Clone the Thor repo:

```sh
git clone https://github.com/vechain/thor.git
cd thor
```

Install dependencies:
(*Note that to make `dep` work, you should put Thor's source code at proper position under your $GOPATH.*)

```sh
dep ensure
```

### Building

To build the main app `thor`, just run

```sh
make
```

or build the full suite:

```sh
make all
```

If no error reported, all built executable binaries will appear in folder *bin*.

## Running Thor

Connect to VeChain's mainnet:

```sh
bin/thor
```

To find out usages of all command line options:

```sh
bin/thor -h
```

- `--dir value`          main directory for configs and databases
- `--dev`                develop mode
- `--beneficiary value`  address for block rewards
- `--api-addr value`     API service listening address (default: "localhost:8669")
- `--api-cors value`     comma separated list of domains from which to accept cross origin requests to API
- `--verbosity value`    log verbosity (0-9) (default: 3)
- `--max-peers value`    maximum number of P2P network peers (P2P network disabled if set to 0) (default: 10)
- `--p2p-port value`     P2P network listening port (default: 11235)
- `--nat value`          port mapping mechanism (any|none|upnp|pmp|extip:<IP>) (default: "none")
- `--help, -h`           show help
- `--version, -v`        print the version


## FAQ

## Community

## Contributing

## License

VeChain Thor is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.html), also included
in *COPYING* file in repository.
