## Thor
Thor block chain 官方的 golang 实现.

## Building the source
```
git clone https://github.com/vechain/thor
```

构建 thor 需要 Go（1.8或更高版本）和 C 编译器, 请自行安装, 然后到项目根目录下执行:

```
make
```

这将在项目根目录下生成 `bin/thor` 可执行文件.

`bin/thor` 是我们的主要 CLI 客户端, 它是进入 thor 网络（主网络, Dev 网络）的入口点并提供 restful 服务以供第三方程序使用.

## Running thor
`$ thor` 命令将启动 CLI 客户端并尝试连入 thor 网络, 成功接入后它将同步本地节点与 thor 网络的数据 (这可能需要花费一点时间).

当然, 可以通过指定命令行参数来自定义 thor 的行为. `thor --help` 查看所有的命令行参数:

- `--p2paddr value` p2p listen addr (default: ":11235")
- `--apiaddr value` restful addr (default: "127.0.0.1:8669")
- `--datadir value` chain data path (default: "/tmp/thor-data")
- `--verbosity value` log verbosity (0-9) (default: 3)
- `--devnet` develop network
- `--beneficiary value` address of beneficiary
- `--maxpeers value` maximum number of network peers (network disabled if set to 0) (default: 10)
- `--help, -h` show help
- `--version, -v` print the version
