package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/vecore/cry"
	"github.com/vechain/vecore/kv"
	State "github.com/vechain/vecore/state"
	"github.com/vechain/vecore/vcc"
)

func main() {
	opt := kv.Options{CacheSize: 10, OpenFilesCacheCapacity: 10}
	db, _ := kv.New("/Users/dinn/Desktop/db", opt)
	hash, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	s, _ := State.NewState(*hash, db)
	accountManager := &vcc.AccountManager{
		State: s,
	}
	fmt.Printf("server start")
	router := mux.NewRouter()
	vcc.NewHTTPRouter(router, accountManager)
	http.ListenAndServe(":3000", router)
	fmt.Printf("server listen 3000")
}
