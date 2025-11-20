module github.com/vechain/thor/v2

go 1.24.2

require (
	github.com/beevik/ntp v0.2.0
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
	github.com/dop251/goja v0.0.0-20230707174833-636fdf960de1
	github.com/elastic/gosigar v0.10.5
	github.com/ethereum/go-ethereum v1.8.14
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/golang-lru v0.0.0-20160813221303-0a025b7e63ad
	github.com/holiman/uint256 v1.2.4
	github.com/mattn/go-isatty v0.0.3
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/mattn/go-tty v0.0.0-20180219170247-931426f7535a
	github.com/pborman/uuid v0.0.0-20170612153648-e790cca94e6c
	github.com/pkg/errors v0.8.1-0.20171216070316-e881fd58d78e
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v1.18.0
	github.com/prometheus/client_model v0.5.0
	github.com/prometheus/common v0.45.0
	github.com/qianbin/directcache v0.9.7
	github.com/qianbin/drlp v0.0.0-20240102101024-e0e02518b5f9
	github.com/stretchr/testify v1.10.0
	github.com/syndtr/goleveldb v1.0.1-0.20220614013038-64ee5596c38a
	github.com/vechain/go-ecvrf v0.0.0-20251023142748-481dd12dec86
	golang.org/x/crypto v0.45.0
	golang.org/x/sync v0.18.0
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/urfave/cli.v1 v1.20.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aristanetworks/goarista v0.0.0-20180222005525-c41ed3986faa // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcd v0.0.0-20171128150713-2e60448ffcc6 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.4 // indirect
	github.com/cespare/cp v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/deckarep/golang-set v1.7.1 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/fatih/color v1.7.0 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/go-stack/stack v1.7.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/huin/goupnp v0.0.0-20171109214107-dceda08e705b // indirect
	github.com/jackpal/go-nat-pmp v1.0.2-0.20160603034137-1fa385a6f458 // indirect
	github.com/mattn/go-colorable v0.0.9 // indirect
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rjeczalik/notify v0.9.3 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/karalabe/cookiejar.v2 v2.0.0-20150724131613-8dcd6a7f4951 // indirect
)

replace github.com/syndtr/goleveldb => github.com/vechain/goleveldb v1.0.1-0.20220809091043-51eb019c8655

replace github.com/ethereum/go-ethereum => github.com/vechain/go-ethereum v1.8.15-0.20250708104014-34fea45fc2b7
