## `log` package

This directory is a copy from  [go-ethereum](https://github.com/ethereum/go-ethereum/tree/v1.14.4) v1.14.4, with some modifications:
- Added geth license headers to all go files
- Changed [root.go](./root.go) `func New(ctx ...interface{})` to return a RootWithContext which allows for global initialization of the logger
