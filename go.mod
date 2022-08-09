module github.com/vechain/thor

go 1.16

require (
	github.com/aristanetworks/goarista v0.0.0-20180222005525-c41ed3986faa // indirect
	github.com/beevik/ntp v0.2.0
	github.com/btcsuite/btcd v0.0.0-20171128150713-2e60448ffcc6 // indirect
	github.com/cespare/cp v1.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
	github.com/elastic/gosigar v0.10.5
	github.com/elazarl/go-bindata-assetfs v1.0.0
	github.com/ethereum/go-ethereum v1.8.14
	github.com/fatih/color v1.7.0 // indirect
	github.com/go-stack/stack v1.7.0 // indirect
	github.com/gorilla/context v0.0.0-20160226214623-1ea25387ff6f // indirect
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.6.0
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/golang-lru v0.0.0-20160813221303-0a025b7e63ad
	github.com/holiman/uint256 v1.2.0
	github.com/huin/goupnp v0.0.0-20171109214107-dceda08e705b // indirect
	github.com/inconshreveable/log15 v0.0.0-20171019012758-0decfc6c20d9
	github.com/jackpal/go-nat-pmp v1.0.1 // indirect
	github.com/mattn/go-colorable v0.0.9 // indirect
	github.com/mattn/go-isatty v0.0.3
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/mattn/go-sqlite3 v1.14.9
	github.com/mattn/go-tty v0.0.0-20180219170247-931426f7535a
	github.com/pborman/uuid v0.0.0-20170612153648-e790cca94e6c
	github.com/pkg/errors v0.8.0
	github.com/pmezard/go-difflib v1.0.0
	github.com/qianbin/directcache v0.9.6
	github.com/rjeczalik/notify v0.9.1 // indirect
	github.com/stretchr/testify v1.7.2
	github.com/syndtr/goleveldb v1.0.1-0.20220614013038-64ee5596c38a
	github.com/vechain/go-ecvrf v0.0.0-20220525125849-96fa0442e765
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/karalabe/cookiejar.v2 v2.0.0-20150724131613-8dcd6a7f4951 // indirect
	gopkg.in/urfave/cli.v1 v1.20.0
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/syndtr/goleveldb => github.com/vechain/goleveldb v1.0.1-0.20220809091043-51eb019c8655

replace github.com/ethereum/go-ethereum => github.com/vechain/go-ethereum v1.8.15-0.20220606031836-4784dac628d7
