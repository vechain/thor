## `log` package

This directory is a copy from  [go-ethereum](https://github.com/ethereum/go-ethereum/tree/v1.14.4) v1.14.4, with some modifications:
- Added geth license headers to all go files
- Changed [root.go](./root.go) `func New(ctx ...interface{})` to `func WithContext(ctx ...interface{}) Logger` to allow for global initialization of the logger.
- Exported legacy log levels for flag documentation
- Removed `./handler_glog.go` and modified tests
