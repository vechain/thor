## VeChainThor

Thor is VeChain's new generation blockchain project.  It's the official implementation written in golang.

[![Go Report Card](https://goreportcard.com/badge/github.com/vechain/thor)](https://goreportcard.com/report/github.com/vechain/thor)
[![Travis](https://travis-ci.org/vechain/thor.svg?branch=master)](https://travis-ci.org/vechain/thor)

## Installation

### Requirements

Thor requires `Go` 1.10+ and `C` compiler to build. To install `Go`, follow this [link](https://golang.org/doc/install). 


### Getting the source

Clone the Thor repo:

```
git clone https://github.com/vechain/thor.git
cd thor
```


### Dependency management

Simply run:
```
make dep
```

To manually install dependencies, choices are

- [dep](https://github.com/golang/dep), Golang's official dependency management tool 

    ```
    dep ensure
    ```
    (*Note that to make `dep` work, you should put the source code at `$GOPATH/src/github.com/vechain/thor`*)

- git submodule

    ```
    git submodule update --init
    ```

### Building

To build the main app `thor`, just run

```
make
```

or build the full suite:

```
make all
```

If no error reported, all built executable binaries will appear in folder *bin*.

## Running Thor

Connect to VeChain's testnet:

```
bin/thor -network test
```


To find out usages of all command line options:

```
bin/thor -h
```

- `--network value`      the network to join (test)
- `--data-dir value`     directory for block-chain databases
- `--beneficiary value`  address for block rewards
- `--api-addr value`     API service listening address (default: "localhost:8669")
- `--api-cors value`     comma separated list of domains from which to accept cross origin requests to API
- `--verbosity value`    log verbosity (0-9) (default: 3)
- `--max-peers value`    maximum number of P2P network peers (P2P network disabled if set to 0) (default: 25)
- `--p2p-port value`     P2P network listening port (default: 11235)
- `--nat value`          port mapping mechanism (any|none|upnp|pmp|extip:<IP>) (default: "none")
- `--help, -h`           show help
- `--version, -v`        print the version

### Sub-commands

- `solo`                client runs in solo mode for test & dev

```
bin/thor solo --on-demand               # create new block when there is pending transaction
bin/thor solo --persist                 # save blockchain data to disk(default to memory)
bin/thor solo --persist --on-demand     # two options can work together
```

- `master-key`          import and export master key

```
# export master key to keystore
bin/thor master-key --export > keystore.json


# import master key from keystore
cat keystore.json | bin/thor master-key --import
```


## Testnet faucet

``` 
curl -X POST -d '{"to":"Your_Address"}' -H "Content-Type: application/json" https://faucet.outofgas.io/requests
```

## API

Once `thor` started, online *OpenAPI* doc can be accessed in your browser. e.g. http://localhost:8669/ by default.


## FAQ

## Acknowledgement

## Community

## Contributing

## License

VeChainThor is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.html), also included
in *LICENSE* file in repository.
