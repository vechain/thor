module github.com/vechain/thor

go 1.12

require (
	github.com/aristanetworks/goarista v0.0.0-20180222005525-c41ed3986faa // indirect
	github.com/beevik/ntp v0.2.0
	github.com/cespare/cp v1.1.1 // indirect
	github.com/coocood/freecache v1.1.1-0.20191203093230-cf06d5fa0ac1
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1 // indirect
	github.com/elastic/gosigar v0.10.5
	github.com/elazarl/go-bindata-assetfs v1.0.0
	github.com/ethereum/go-ethereum v1.9.25
	github.com/fatih/color v1.7.0 // indirect
	github.com/gorilla/context v0.0.0-20160226214623-1ea25387ff6f // indirect
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.6.0
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/golang-lru v0.5.4
	github.com/inconshreveable/log15 v0.0.0-20171019012758-0decfc6c20d9
	github.com/mattn/go-isatty v0.0.5-0.20180830101745-3fb116b82035
	github.com/mattn/go-sqlite3 v1.14.1
	github.com/mattn/go-tty v0.0.0-20180219170247-931426f7535a
	github.com/pborman/uuid v0.0.0-20170612153648-e790cca94e6c
	github.com/pkg/errors v0.8.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/stretchr/testify v1.4.0
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/sys v0.0.0-20200824131525-c12d262b63d8
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/karalabe/cookiejar.v2 v2.0.0-20150724131613-8dcd6a7f4951
	gopkg.in/olebedev/go-duktape.v3 v3.0.0-20200619000410-60c24ae608a6
	gopkg.in/urfave/cli.v1 v1.20.0
	gopkg.in/yaml.v2 v2.3.0
)

replace github.com/syndtr/goleveldb => github.com/vechain/goleveldb v1.0.1-0.20200918014306-20f0a95f6dd4
