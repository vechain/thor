# VeChain Thor JSON-RPC Server —— 设计决策与 Bootstrap 实施计划

> 本文合并了三部分内容:**为什么这么做**(设计决策)、**参照什么做**(go-ethereum 反射注册契约提炼)、**具体怎么做**(可运行的 bootstrap 实施计划)。
>
> 目标读者:审阅设计合理性的评审者,以及照此落地的实现者。范围严格限定在 **bootstrap**——把 JSON-RPC server 骨架 + 反射注册跑起来,用一两个方法证明整条链路通;不覆盖完整方法集、WS、订阅、filter、写入路径(见 §1.3 Non-goals)。

---

## 1. 背景与目标

### 1.1 Thor 当前 API 形态

- 协议:**REST over HTTP**(路由由 `gorilla/mux` 维护),订阅走 **WebSocket frame**(自定义协议,非 JSON-RPC 2.0)。
- 入口:`api/` 目录,每个子模块(`accounts/`、`blocks/`、`transactions/`、`events/`、`debug/`、`fees/`、`node/`、`subscriptions/` …)独立一个包,自带 `Mount(router, "/path")` 风格的路由注册。
- 没有反射注册、没有 namespace 概念,URL path 即"方法名"。

### 1.2 目标

引入一套以太坊兼容的 JSON-RPC 2.0 server,让 `cast` / MetaMask / ethers / viem 等标准客户端能直接连 thor 节点。本文交付其中的**最小可运行内核 + 一两个示例方法**,并把设计决策一并说清,便于评审。

### 1.3 范围

**In scope(本次交付)**

1. 自研的最小 JSON-RPC 2.0 server 内核,**核心是反射注册**(照搬 geth `rpc/service.go` 契约,见 §3.2)。
2. HTTP transport:`POST /rpc`,支持单请求 + 批量数组。
3. 最小错误模型(`-32600/-32601/-32602/-32603/-32000` + `DataError` 接口)。
4. 挂载:在 `cmd/thor/httpserver/api_server.go` 加一行 `.Mount(router, "/rpc")`,复用 thor 现有中间件链。
5. 示例方法:`eth_chainId`、`eth_blockNumber`(两个零参主演示)+ 附赠 `eth_getBalance`(演示带参解码 + 读 state)。

**Non-goals(明确推迟)**

| 推迟项 | 去向 |
|---|---|
| WebSocket transport / `eth_subscribe` / `Notifier` | Phase 2 |
| filter API(`eth_newFilter` 等) | Phase 2 |
| 完整只读集(`eth_call` / `getBlockByNumber` / receipt / logs …) | Phase 1/2 |
| 写入路径(`eth_sendRawTransaction`) | Phase 2 |
| batch item/size 上限、method timeout 精细化 | 先给保守默认值,Phase 2 调优 |
| 完整 `BlockNumberOrHash` union + ethereum↔thor revision 双向映射 | Phase 1;bootstrap 里 `eth_getBalance` 只支持 `latest`/缺省=best,其余标 TODO |
| `--rpc-modules` 等运维配置 | Phase 1;bootstrap 只加一个 `--enable-rpc` 开关 |
| IPC / inproc / stdio / JWT Auth 端口 | 不做 |

### 1.4 验收标准

- `go build ./...` 通过;`go test ./api/jsonrpc/...` 通过。
- 节点启动后:
  ```
  curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'
  # => {"jsonrpc":"2.0","id":1,"result":"0x..."}
  ```
- 未知方法返回 `-32601`;参数错误 `-32602`;方法内 panic 被 recover 成 `-32603`(各有单测)。

---

## 2. 设计决策

### 2.1 依赖现实 → 排除"直接 import geth rpc 包"

thor 当前依赖(已在本分支 `go.mod` 核实):

```
require  github.com/ethereum/go-ethereum v1.8.14
replace  github.com/ethereum/go-ethereum => github.com/vechain/go-ethereum v1.8.15-0.20260511103518-...
```

这是 **2018 年的 fork**,与当前 go-ethereum 主干(v1.16+)差 7 年以上。那个版本的 `rpc/` 包没有现代反射结构、`handler.go` 状态机、`Notifier.activate` 缓冲、`SetBatchLimits` 等机制。更关键:thor 现有代码**根本没有 import `go-ethereum/rpc`**(全仓 `grep` 0 命中),只用 `common` / `crypto` / `core/types` / `rlp` / `event` / `log` 等子包,全部锚定 1.8 分支。

**含义**:任何"直接 `import go-ethereum/rpc`"的方案都不可行——要让现代 rpc 包可用,必须整体升级 go-ethereum 依赖跨越 7 年版本(`core/types` / `core/state` / `log` 均有破坏性变更,需重新移植 thor 全部 patch,估 2–4 个工程师月),远超 RPC 子系统本身工作量,不在本课题范围。反过来,`rpc` 包 0 引用也意味着**自研内核不会与任何现有 import 冲突**。

### 2.2 方案对比与结论

| 选项 | 一句话 | 评估 |
|---|---|---|
| A. 直接 import geth `rpc` 包 | `import ".../rpc"` | **不可行**:依赖 1.8 fork,无现代 rpc 包结构(§2.1) |
| B. Fork `rpc` 包并裁剪 | 拷现代 geth `rpc/` 进 thor 按需精简 | 可行但留死代码:8k 行里 HTTP-only+子路径场景只用得到 ~30%,裁剪约 2.5 周,与自研工作量相当 |
| C. 第三方库 | `creachadair/jrpc2`、`gorilla/rpc` | **不推荐**:钱包/dApp 兼容要求订阅/错误码/batch 逐字节对齐 geth,逐项校准成本≈自研,还多一个外部依赖 |
| **D. 自研最小实现** | `api/jsonrpc/` 写最小 JSON-RPC 2.0 server,参照 geth 设计 | **推荐**:HTTP+子路径场景 ~1.5k 行覆盖;反射+namespace 注册照搬 geth |
| E. 旁路代理/网关 | thor 外跑翻译进程 | 仅作过渡:订阅一致性 + 延迟是硬指标,且"不算在 vechain 内" |
| **F. 子路径挂载** | 作为 thor API server 子模块挂在 `/rpc` | **推荐拓扑**:与 REST 同端口同 router,复用全部中间件 |

**结论:D + F**——自研最小 JSON-RPC server,挂在 thor API 端口的 `/rpc` 子路径下。

### 2.3 拓扑:进程内 `/rpc` 子路径 + HTTP-only

沿用 thor 现有 `.Mount(router, "/path")` 模式,挂在 `/rpc`:

```go
jsonrpc.New(deps).Mount(router, "/rpc")
```

选择依据:

| 维度 | 价值 |
|---|---|
| 端口数 | 1 个(thor API 端口 :8669),无新端口、无新 TLS |
| 中间件 | **完全复用** thor 现有 body-limit / timeout / request-logger / metrics / panic / CORS / gzip,零成本继承 |
| 客户端 URL | `http://host:8669/rpc`(未来 WS `ws://host:8669/rpc`,仅 scheme 不同) |
| 与 geth 习惯一致 | geth `httpServer` 也在同一路径上靠 `isWebsocket(r)` 区分 HTTP/WS |
| 实现复杂度 | 一个 `.Mount(router, "/rpc")` 调用 |

**已排除**:独立端口(要新增 `--rpc-port`/`--rpc-cors`/`--rpc-vhost` 并重接中间件,与 thor 架构不贴合,且 thor 无 geth "Engine API 独立 Auth 端口"需求);按 `Content-Type` body-sniffing 分发(太魔法,调试困难)。

> **WS 的位置**:WebSocket 握手是带 `Upgrade: websocket` 的 HTTP GET,可与 `POST /rpc` 共用同一路径。bootstrap **只做 HTTP**(`POST /rpc`);Phase 2 在同一 `/rpc` 子路由上加一条 `GET + Upgrade` 路由即可,业务方法在 transport 层无感。§5.5 的 `Mount` 已标注插入点。

### 2.4 集成不变量(引入 JSON-RPC 时必须守住)

1. **REST 接口不动**:保留所有现有路径与语义,作为 VeChain 原生 API。两套接口长期并存。
2. **底层数据源必须同一**:JSON-RPC 与 REST 共享同一 `chain.Repository` / `txpool.TxPool` / `state.Stater`,**禁止任一侧建立独立缓存或快照副本**——否则两套接口下链状态会分裂。bootstrap 里通过复用 `restutil.GetSummaryAndState` 落实这条(§5.6)。
3. **写入路径合并**(Phase 2):JSON-RPC 写入方法与 REST `POST /transactions` 最终都走 `txpool.Add(tx)`。

### 2.5 共享中间件链对 `/rpc` 的副作用(重要)

`/rpc` 挂在同一 `mux.Router` 上,`router.Use(...)` 的中间件**全套生效**(顺序,由外到内:CORS → body-limit → timeout → request-logger → metrics → panics → x-genesis-id → x-thorest-ver → gzip → 路由)。已逐个核对 `api/middleware/`,对 JSON-RPC 语义的影响:

| 中间件 | 对 JSON-RPC 的影响 | bootstrap 对策 |
|---|---|---|
| **HandleXGenesisID** | 只在 client **显式传了不匹配的** `x-genesis-id` 时返回 403;缺省不传则放行 → cast / MetaMask / ethers / viem 不受影响。每个响应会多一个 `x-genesis-id` 头(eth client 忽略) | ✅ 无需处理 |
| **HandleRequestBodyLimit(200KB)** | 读满即 `MaxBytesReader` 截断 → `io.ReadAll` 收到错误;**限制了批量请求体上限**。超限时写的是**纯文本 413**,非 JSON-RPC 错误 | 可接受;大 batch 会被拒,Phase 2 若需要再放宽 |
| **RequestLoggerMiddleware** | **已正确 read + restore body**(`io.NopCloser(bytes.NewReader(bodyBytes))`),不会吞掉 handler 要读的 body ✅。但启用时会**把整个 JSON-RPC 请求体记日志**(未来含 `eth_sendRawTransaction` 原始签名交易);URI 恒为 `POST /rpc`、批量里 N 个调用并成 1 条日志,**method 名不可见** | 功能正确,可接受;要 per-method 日志则 Phase 1+ 加 JSON-RPC 感知字段 |
| **MetricsMiddleware** | 按路由(`.Name("POST /rpc")`)聚合 → **所有 JSON-RPC 调用共用一个 label**,method 维度丢失,批量掩盖真实调用数 | 观测粒度问题,非正确性;Phase 1+ 需按 method 统计则自建指标 |
| **HandleAPITimeout**(仅 `config.Timeout>0`) | 给 request ctx 加 deadline 后继续,**本身不写 503**,只靠 ctx 取消;方法若不检查 ctx 就跑到底 | ✅ 有益(每请求 deadline);但**未映射成 `-32002`**,Phase 1+ 让方法 honor ctx 并转 -32002 |
| **HandlePanics** | 兜底 recover 后写**纯文本 500**,非 JSON-RPC 错误。dispatch 已 per-method recover 成 `-32603`,此兜底仅在 codec 层 panic 时触发 | 保持 handler 自身 panic-safe,尽量不触发 |
| **gzip / CORS** | gzip 透明;CORS 已 `AllowedHeaders("content-type")`,JSON-RPC 预检通过。浏览器 dApp 需运维配 `--api-allowed-origins`(与 REST 现状一致),node 客户端不受 CORS 约束 | ✅ 无需处理 |

**共性问题**:body-limit(413)、genesis 不匹配(403)、panic(500)、timeout 这几条**错误出口写的都是纯 HTTP/文本响应,不是 `-326xx` JSON-RPC 2.0 错误信封**。标准 eth 客户端会把它们当成不透明的传输层错误而非 JSON-RPC error 对象。bootstrap 可接受;严格一致性(Phase 1+)需要在 `/rpc` 这条链上把这些出口也包成 JSON-RPC 错误信封。

**Phase 2 加 WS 前必须先拆的雷**(现在记下,免遗漏):

- `http.Server{ReadTimeout: 5s}` 是**全局**的,会掐断长连接 WS → 需对 `/rpc` 的 WS upgrade 单独处理读超时。
- `RequestLoggerMiddleware` 慢查询豁免目前是 `!HasPrefix(path, "/subscriptions")`,需把 `/rpc` 的 WS upgrade **一并豁免**。
- `HandleAPITimeout` 会给 WS 连接 ctx 加 deadline,需对 WS upgrade **豁免**。

---

## 3. 参考蓝本:go-ethereum 反射注册契约

自研内核照 geth `rpc/` 的设计,但只取 HTTP-only bootstrap 用得到的部分。以下是提炼后的契约(来源:go-ethereum `rpc/service.go` / `rpc/http.go` / `rpc/errors.go`)。

### 3.1 两个核心抽象

- **`ServerCodec`**:一个 JSON-RPC 双向消息通道(read batch / write JSON)。HTTP、(未来)WS 各实现一个 codec,业务核心无感。bootstrap 只有 HTTP 一个 codec。
- **`serviceRegistry`**:用反射把任意 Go struct 的导出方法挂成 `<namespace>_<method>` 形式的 RPC 端点。一个 `Server` 实例可被多个 transport 复用。

```
Transport+Codec  ──JSON-RPC 2.0──▶  rpc.Server  ──reflect──▶  Go method on user struct
```

### 3.2 反射注册契约(★核心)

任何 Go struct 调用一次 `Server.RegisterName("eth", svc)` 后:反射扫描其所有**导出**方法,逐个校验签名,合格者以"方法名首字母小写"入注册表,最终经 `<namespace>_<methodName>` 暴露(`RegisterName("eth", ...)` 的 `GetBalance` → `eth_getBalance`)。

**普通 RPC 方法的合法签名(四选一)**:

```go
func (s *Svc) Method() error
func (s *Svc) Method(args...) Result
func (s *Svc) Method(args...) (Result, error)
func (s *Svc) Method(ctx context.Context, args...) (Result, error)
```

**注册方契约清单**:

1. **namespace 非空**;`rcvr` 通常传 `*T`(带状态);同一 namespace **允许多次注册**(第二次把新方法 merge 进同一棵 map)——geth 借此把 `eth` 拆给多个 struct 共同贡献,是"按业务关注点拆类"与"对外同一 namespace"解耦的关键。
2. **只有导出方法**(`PkgPath == ""`)会被注册;小写方法静默跳过。
3. **返回值 ≤ 2**;若有 `error` 必须放最后(`(error, T)` 不合法)。
4. **首个参数若是 `context.Context`,框架自动注入**,不计业务参数;客户端不传 ctx。
5. **业务参数/返回值类型必须能被 `encoding/json` 编解码**:大整数用 `*hexutil.Big`、字节用 `hexutil.Bytes`、地址 `common.Address` 等带 JSON 编解码方法的类型;不要用 `chan`/`func`;**不要返回裸 `*big.Int`**(会被序列化成十进制字符串)。
6. **构造函数注入依赖**:访问 backend/chain/txpool 靠 struct 字段持有,由 `NewXxx(deps...)` 传入。
7. **不要 panic**:框架会 recover 并转 `-32603`,但日志会爆且对客户端不友好。要附结构化错误信息就实现 `DataError`(§3.3)。
8. **并发**:框架在 per-connection 内串行调用,但**跨连接并发**,业务 struct 必须自管 lock。

> 订阅方法有其唯一签名 `func (s *Svc) Sub(ctx, args...) (*Subscription, error)`,并只在 WS/IPC 生效——bootstrap 不做,Phase 2 再引入。

### 3.3 错误模型

错误码照搬 geth 语义:

| 错误码 | 触发场景 |
|---|---|
| `-32700` | JSON 解析失败(parse error) |
| `-32600` | 缺字段 / 字段类型错(invalid request) |
| `-32601` | 方法未注册(method not found) |
| `-32602` | 参数反序列化失败(invalid params) |
| `-32603` | 业务方法 panic 或响应编码失败(internal error,dispatch 层 recover) |
| `-32000` | 通用业务错误(vechain 自有错误统一映射到此) |

业务错误通过 `DataError` 接口把结构化字段塞进 `error.data`(EVM revert reason、过期 BlockRef 等):

```go
type DataError interface {
    error
    ErrorCode() int
    ErrorData() interface{}
}
```

### 3.4 HTTP transport 行为要点

- HTTP 总是**单次请求**:一次 `POST` 处理完即关闭 codec——**HTTP 不支持订阅**(HTTP 上调 `eth_subscribe` 应返回错误,而非静默成功)。
- 用 `json.Decoder.UseNumber()`(或等价手段)避免大整数被解析成 float64 丢精度。
- body 大小限制:bootstrap 直接靠 thor 外层 `HandleRequestBodyLimit`(200KB)兜底;batch item/size 精细上限留 Phase 2。

---

## 4. 当前分支代码锚点(实测)

以下均已在当前 `json-rpc` 分支核对(模块路径 `github.com/vechain/thor/v2`):

- **装配点**:`cmd/thor/httpserver/api_server.go` 的 `StartAPIServer(...)`,`router := mux.NewRouter()`,各子包以 `New(deps...).Mount(router, "/path")` 注册(`accounts`/`blocks`/`transactions`/`debug`/`node`/`fees`/`subscriptions`)。
- **子包约定**:handler 签名 `func(w http.ResponseWriter, r *http.Request) error`,用 `restutil.WrapHandlerFunc` 包成 `http.HandlerFunc`;响应用 `restutil.WriteJSON(w, obj)`,入参用 `restutil.ParseJSON`,错误用 `restutil.BadRequest(err)` / `restutil.HTTPError(cause, status)`。共享 DTO 放顶层 `api` 包的 `*_types.go`。
- **中间件**:`api/middleware/`(body-limit / timeout / request-logger / metrics / panic / x-genesis-id / x-thorest-ver)+ CORS + gzip。JSON-RPC 挂同一 router 即全部继承。
- **可用类型**:`github.com/ethereum/go-ethereum/common/hexutil`、`common.Address` / `common.Hash` 均可 import(仓库已大量使用)。`go-ethereum/rpc` **全仓 0 引用**。
- **域访问器(示例方法要用的)**:
  - `repo.ChainID() uint64`(`chain/repository.go`——已是 EL 兼容 chain id,取自 genesis id `[30:32]`)。
  - `repo.BestBlockSummary().Header.Number() uint32`。
  - `thor.Address = type Address common.Address`(20 byte),`common.Address(addr)` 零成本互转;`thor.Bytes32 = [32]byte`。
  - state:**没有 `StateAt`**。先 `restutil.ParseRevision(s string, allowNext bool) (*Revision, error)`(缺省/`"best"` = best),再 `restutil.GetSummaryAndState(rev *Revision, repo, bft, stater, forkConfig) (*chain.BlockSummary, *state.State, error)`(`api/restutil/revisions.go`),最后 `state.GetBalance(thor.Address) (*big.Int, error)`。

---

## 5. Bootstrap 实施

### 5.1 包结构

```
api/jsonrpc/
├── jsonrpc.go        // 入口:New(deps) *JSONRPC;Mount(router, "/rpc");HTTP handler
├── server.go         // Server:注册表容器 + 单条消息分发(handleMsg)
├── service.go        // ★反射注册内核(callback / serviceRegistry / RegisterName)
├── json.go           // JSON-RPC 2.0 envelope + 错误类型(jsonError / DataError)
├── backend.go        // 最小 Backend:持有 repo/stater/bft/forkConfig
├── eth.go            // 示例 namespace:eth_chainId / eth_blockNumber / eth_getBalance
├── net.go            // net_version(第二 namespace,证明多 namespace 注册)
├── web3.go           // web3_clientVersion
└── jsonrpc_test.go   // 反射签名 / 分发 / 错误码 单测
```

> `net.go`/`web3.go` 是可选点缀(各一个方法),证明"多 namespace、同一 Server 实例"。想更省可只留 `eth.go`。

### 5.2 ★反射注册内核(`service.go`)

```go
// api/jsonrpc/service.go
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// callback 封装一个已注册 service 的单个导出方法。
type callback struct {
	rcvr     reflect.Value  // 方法的 receiver
	fn       reflect.Value  // 方法本体(method.Func,In(0) 是 receiver)
	argTypes []reflect.Type // 业务入参类型(不含 receiver / ctx)
	hasCtx   bool           // 首个非 receiver 参数是否为 context.Context
	errPos   int            // error 返回值下标;-1 表示无 error 返回
}

// serviceRegistry: namespace -> methodName -> *callback(两层 map,照搬 geth)。
type serviceRegistry struct {
	mu       sync.Mutex
	services map[string]map[string]*callback
}

// registerName 反射扫描 rcvr 的导出方法并入表。同一 namespace 允许多次注册(合并)。
func (r *serviceRegistry) registerName(namespace string, rcvr interface{}) error {
	if namespace == "" {
		return fmt.Errorf("jsonrpc: namespace cannot be empty")
	}
	callbacks := suitableCallbacks(reflect.ValueOf(rcvr))
	if len(callbacks) == 0 {
		return fmt.Errorf("jsonrpc: service %T has no suitable methods to expose", rcvr)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.services == nil {
		r.services = make(map[string]map[string]*callback)
	}
	svc := r.services[namespace]
	if svc == nil {
		svc = make(map[string]*callback)
		r.services[namespace] = svc
	}
	for name, cb := range callbacks { // merge:同 namespace 二次注册追加方法
		svc[name] = cb
	}
	return nil
}

// callback 按 "<namespace>_<method>" 查找。
func (r *serviceRegistry) callback(method string) *callback {
	i := strings.IndexByte(method, '_')
	if i <= 0 {
		return nil
	}
	ns, name := method[:i], method[i+1:]
	r.mu.Lock()
	defer r.mu.Unlock()
	if svc := r.services[ns]; svc != nil {
		return svc[name]
	}
	return nil
}

// suitableCallbacks 扫描导出方法,跳过不符合签名契约的(静默,与 geth 一致)。
func suitableCallbacks(receiver reflect.Value) map[string]*callback {
	typ := receiver.Type()
	out := make(map[string]*callback)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		if method.PkgPath != "" { // 未导出
			continue
		}
		if cb := newCallback(receiver, method.Func); cb != nil {
			out[formatName(method.Name)] = cb // GetBalance -> getBalance -> eth_getBalance
		}
	}
	return out
}

// newCallback 校验方法签名,合法则构造 callback,否则返回 nil(跳过)。
func newCallback(receiver, fn reflect.Value) *callback {
	fnType := fn.Type()

	// 返回值:<=2;若 2 个则第 2 个必须是 error;若 1 个可为 error 或 result。
	errPos := -1
	switch fnType.NumOut() {
	case 0:
	case 1:
		if isErrorType(fnType.Out(0)) {
			errPos = 0
		}
	case 2:
		if !isErrorType(fnType.Out(1)) {
			return nil // 第二返回值必须是 error
		}
		errPos = 1
	default:
		return nil // 超过 2 个返回值
	}

	// 入参:In(0)=receiver;若 In(1)=context.Context 则自动注入、不计业务参数。
	hasCtx := false
	firstArg := 1
	if fnType.NumIn() > 1 && fnType.In(1) == contextType {
		hasCtx = true
		firstArg = 2
	}
	argTypes := make([]reflect.Type, 0, fnType.NumIn()-firstArg)
	for i := firstArg; i < fnType.NumIn(); i++ {
		argTypes = append(argTypes, fnType.In(i))
	}
	return &callback{rcvr: receiver, fn: fn, argTypes: argTypes, hasCtx: hasCtx, errPos: errPos}
}

// parseArgs 把 JSON 数组按位置解码成实参。缺省的尾参取零值(可选参数)。
func (c *callback) parseArgs(rawParams json.RawMessage) ([]reflect.Value, error) {
	if len(c.argTypes) == 0 {
		return nil, nil
	}
	var params []json.RawMessage
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &jsonError{Code: errcodeInvalidParams, Message: err.Error()}
		}
	}
	if len(params) > len(c.argTypes) {
		return nil, &jsonError{Code: errcodeInvalidParams,
			Message: fmt.Sprintf("too many arguments, want at most %d", len(c.argTypes))}
	}
	args := make([]reflect.Value, len(c.argTypes))
	for i, t := range c.argTypes {
		if i < len(params) && len(params[i]) > 0 && string(params[i]) != "null" {
			val := reflect.New(t)
			if err := json.Unmarshal(params[i], val.Interface()); err != nil {
				return nil, &jsonError{Code: errcodeInvalidParams,
					Message: fmt.Sprintf("invalid argument %d: %v", i, err)}
			}
			args[i] = val.Elem()
		} else {
			args[i] = reflect.Zero(t) // 缺省/null -> 零值
		}
	}
	return args, nil
}

// call 真正的反射 invoke;panic 一律 recover 成 -32603,不冲垮连接。
func (c *callback) call(ctx context.Context, args []reflect.Value) (res interface{}, errRes error) {
	full := make([]reflect.Value, 0, 2+len(args))
	full = append(full, c.rcvr)
	if c.hasCtx {
		full = append(full, reflect.ValueOf(ctx))
	}
	full = append(full, args...)

	defer func() {
		if r := recover(); r != nil {
			errRes = &jsonError{Code: errcodeInternal, Message: "method handler crashed"}
		}
	}()

	results := c.fn.Call(full)
	if len(results) == 0 {
		return nil, nil // func() 或 func()error(nil)
	}
	if c.errPos >= 0 && !results[c.errPos].IsNil() {
		return nil, results[c.errPos].Interface().(error)
	}
	if c.errPos == 0 {
		return nil, nil // 只有一个 error 返回值且为 nil
	}
	return results[0].Interface(), nil
}

func isErrorType(t reflect.Type) bool { return t.Implements(errorType) }

func formatName(name string) string { // 首字母小写
	r := []rune(name)
	if len(r) > 0 {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}
```

### 5.3 Envelope 与错误类型(`json.go`)

```go
// api/jsonrpc/json.go
package jsonrpc

import "encoding/json"

const jsonrpcVersion = "2.0"

// JSON-RPC 2.0 错误码(照搬 geth,见 §3.3)。
const (
	errcodeParse          = -32700
	errcodeInvalidRequest = -32600
	errcodeMethodNotFound = -32601
	errcodeInvalidParams  = -32602
	errcodeInternal       = -32603
	errcodeDefault        = -32000 // 通用业务错误
)

type jsonrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonError      `json:"error,omitempty"`
}

type jsonError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *jsonError) Error() string { return e.Message }

// DataError:业务错误把结构化字段塞进 error.data(EVM revert reason / 过期 BlockRef 等)。
type DataError interface {
	error
	ErrorCode() int
	ErrorData() interface{}
}

func errorResponse(id json.RawMessage, je *jsonError) *jsonrpcMessage {
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: id, Error: je}
}

func toJSONError(err error) *jsonError {
	if je, ok := err.(*jsonError); ok {
		return je
	}
	je := &jsonError{Code: errcodeDefault, Message: err.Error()}
	if de, ok := err.(DataError); ok {
		je.Code = de.ErrorCode()
		je.Data = de.ErrorData()
	}
	return je
}
```

### 5.4 Server 分发(`server.go`)

```go
// api/jsonrpc/server.go
package jsonrpc

import "context"

type Server struct {
	registry serviceRegistry
}

func NewServer() *Server { return &Server{} }

func (s *Server) RegisterName(namespace string, rcvr interface{}) error {
	return s.registry.registerName(namespace, rcvr)
}

// handleMsg 处理单条 JSON-RPC 请求,永远返回一条 response(含错误)。
func (s *Server) handleMsg(ctx context.Context, msg *jsonrpcMessage) *jsonrpcMessage {
	if msg.Version != jsonrpcVersion || msg.Method == "" {
		return errorResponse(msg.ID, &jsonError{Code: errcodeInvalidRequest, Message: "invalid request"})
	}
	cb := s.registry.callback(msg.Method)
	if cb == nil {
		return errorResponse(msg.ID, &jsonError{Code: errcodeMethodNotFound,
			Message: "the method " + msg.Method + " does not exist/is not available"})
	}
	args, err := cb.parseArgs(msg.Params)
	if err != nil {
		return errorResponse(msg.ID, toJSONError(err))
	}
	result, err := cb.call(ctx, args)
	if err != nil {
		return errorResponse(msg.ID, toJSONError(err))
	}
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: msg.ID, Result: result}
}
```

### 5.5 入口 + HTTP transport + 挂载(`jsonrpc.go`)

```go
// api/jsonrpc/jsonrpc.go
package jsonrpc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type JSONRPC struct {
	server *Server
}

// New 装配最小 namespace 集合,全部复用同一个 Server 实例(HTTP/未来 WS 共享)。
func New(repo *chain.Repository, stater *state.Stater, bft bft.Committer, forkConfig *thor.ForkConfig) *JSONRPC {
	srv := NewServer()
	b := &backend{repo: repo, stater: stater, bft: bft, forkConfig: forkConfig}

	_ = srv.RegisterName("eth", &ethAPI{b: b}) // 反射注册:一行完成 API 装配
	_ = srv.RegisterName("net", &netAPI{b: b})
	_ = srv.RegisterName("web3", &web3API{})

	return &JSONRPC{server: srv}
}

// Mount 沿用 thor 的 .Mount(root, prefix) 约定,只挂 POST /rpc(HTTP-only,见 §2.3)。
func (j *JSONRPC) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").Methods(http.MethodPost).Name("POST /rpc").
		HandlerFunc(restutil.WrapHandlerFunc(j.handleHTTP))
	// Phase 2:GET /rpc + Upgrade: websocket 在这里加第二条路由(见 §2.3)。
}

// handleHTTP:读 body,区分单请求 / 批量数组,逐条 handleMsg。
func (j *JSONRPC) handleHTTP(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
	}
	ctx := r.Context()
	trimmed := bytes.TrimLeft(body, " \t\r\n")

	if len(trimmed) > 0 && trimmed[0] == '[' { // 批量
		var msgs []jsonrpcMessage
		if err := json.Unmarshal(body, &msgs); err != nil || len(msgs) == 0 {
			return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: "invalid batch"}))
		}
		resps := make([]*jsonrpcMessage, len(msgs))
		for i := range msgs {
			resps[i] = j.server.handleMsg(ctx, &msgs[i])
		}
		return restutil.WriteJSON(w, resps)
	}

	var msg jsonrpcMessage // 单请求
	if err := json.Unmarshal(body, &msg); err != nil {
		return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
	}
	return restutil.WriteJSON(w, j.server.handleMsg(ctx, &msg))
}
```

**装配(`cmd/thor/httpserver/api_server.go`)**:在 import 段加 `"github.com/vechain/thor/v2/api/jsonrpc"`,在 `subscriptions`/`fees` 挂载之后加:

```go
if config.EnableRPC { // 见 §5.7 的开关
	jsonrpc.New(repo, stater, bft, forkConfig).Mount(router, "/rpc")
}
```

`repo` / `stater` / `bft` / `forkConfig` 在 `StartAPIServer` 作用域内均已就绪,无需改签名。

### 5.6 示例方法 + Backend

```go
// api/jsonrpc/eth.go
package jsonrpc

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/thor"
)

type ethAPI struct{ b *backend }

// eth_chainId —— 零参。演示:反射注册 + 无 ctx + (Result, error)。
func (a *ethAPI) ChainId() (*hexutil.Big, error) {
	return (*hexutil.Big)(new(big.Int).SetUint64(a.b.repo.ChainID())), nil
}

// eth_blockNumber —— 零参。演示:hexutil.Uint64 返回类型。
func (a *ethAPI) BlockNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(a.b.repo.BestBlockSummary().Header.Number()), nil
}

// eth_getBalance —— 带位置参数 + ctx + 读 state。演示完整反射入参解码链路。
// 注意:bootstrap 只支持 blockParam 缺省/"latest" = best;完整 BlockNumberOrHash
// union 与 ethereum<->thor revision 双向映射推迟到 Phase 1(见 §1.3 Non-goals)。
func (a *ethAPI) GetBalance(ctx context.Context, addr common.Address, blockParam *string) (*hexutil.Big, error) {
	rev := "best"
	if blockParam != nil && *blockParam != "" && *blockParam != "latest" {
		// TODO(Phase1):映射 earliest/pending/safe/finalized/<hex>/<num> -> thor revision
		rev = *blockParam
	}
	_, st, err := a.b.stateForRevision(rev)
	if err != nil {
		return nil, &jsonError{Code: errcodeDefault, Message: err.Error()}
	}
	bal, err := st.GetBalance(thor.Address(addr)) // common.Address -> thor.Address 零成本
	if err != nil {
		return nil, &jsonError{Code: errcodeDefault, Message: err.Error()}
	}
	return (*hexutil.Big)(bal), nil
}
```

```go
// api/jsonrpc/backend.go
package jsonrpc

import (
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type backend struct {
	repo       *chain.Repository
	stater     *state.Stater
	bft        bft.Committer
	forkConfig *thor.ForkConfig
}

// stateForRevision 复用 REST 的 revision 解析器,保证两套 API 看到同一链状态(§2.4)。
// 注意:GetSummaryAndState 收的是 *restutil.Revision,需先 ParseRevision;allowNext=false。
func (b *backend) stateForRevision(revStr string) (*chain.BlockSummary, *state.State, error) {
	rev, err := restutil.ParseRevision(revStr, false)
	if err != nil {
		return nil, nil, err
	}
	return restutil.GetSummaryAndState(rev, b.repo, b.bft, b.stater, b.forkConfig)
}
```

```go
// api/jsonrpc/net.go
package jsonrpc

import "strconv"

type netAPI struct{ b *backend }

// net_version —— 十进制 chainId 字符串(geth 惯例)。
func (a *netAPI) Version() string {
	return strconv.FormatUint(a.b.repo.ChainID(), 10)
}
```

```go
// api/jsonrpc/web3.go
package jsonrpc

type web3API struct{}

// web3_clientVersion
func (a *web3API) ClientVersion() string {
	return "thor" // Phase 1 换成真实版本串
}
```

> **签名已按本分支核实**:`restutil.WriteJSON(w http.ResponseWriter, obj any) error`、`restutil.HandlerFunc = func(http.ResponseWriter, *http.Request) error`、`restutil.WrapHandlerFunc(HandlerFunc) http.HandlerFunc`、`restutil.GetSummaryAndState(rev *Revision, repo, bft, stater, forkConfig) (*chain.BlockSummary, *state.State, error)`、`restutil.ParseRevision(string, allowNext bool) (*Revision, error)`。上面 `backend.go` 已据此先 `ParseRevision` 再取 state。

### 5.7 配置开关(最小)

1. `APIConfig` 加字段 `EnableRPC bool`(`api_server.go` 的 `APIConfig` 结构)。
2. `cmd/thor` CLI 加 `--enable-rpc`(bool,默认 false),透传进 `APIConfig.EnableRPC`。
3. §5.5 的挂载用 `if config.EnableRPC` 包裹。

不加 `--rpc-port`/`--rpc-cors`/`--rpc-modules`——全部继承 thor API server。纯本地 PoC 也可先无条件挂载、跳过 flag,最后再补开关。

---

## 6. 执行步骤清单(按序)

- [ ] 1. 新建 `api/jsonrpc/` 目录,落 `json.go`(envelope + 错误码 + DataError)。
- [ ] 2. 落 `service.go`(★反射内核 §5.2),先写 `jsonrpc_test.go` 覆盖签名校验(TDD)。
- [ ] 3. 落 `server.go`(`handleMsg` 分发)。
- [ ] 4. 落 `backend.go` + `eth.go` / `net.go` / `web3.go`(示例方法)。
- [ ] 5. 落 `jsonrpc.go`(`New` + `Mount` + `handleHTTP`)。
- [ ] 6. 改 `api_server.go`:import + `APIConfig.EnableRPC` + 一行 `.Mount(router, "/rpc")`。
- [ ] 7. 改 `cmd/thor`:加 `--enable-rpc` flag,透传。
- [ ] 8. `go build ./...` + `go test ./api/jsonrpc/...`。
- [ ] 9. 启动 solo 节点手测三个方法。

---

## 7. 验证

```bash
# 启动(solo 便于本地验证)
make && ./bin/thor solo --on-demand --enable-rpc

# eth_chainId
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'

# eth_blockNumber
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"eth_blockNumber","params":[]}'

# eth_getBalance(best)
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"eth_getBalance","params":["0x0000000000000000000000000000456E65726779"]}'

# 批量
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '[{"jsonrpc":"2.0","id":1,"method":"eth_chainId"},{"jsonrpc":"2.0","id":2,"method":"web3_clientVersion"}]'

# 负例:未知方法 -> -32601
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":9,"method":"eth_nope"}'
```

**单测(`jsonrpc_test.go`)**:反射注册(合法/非法签名各一)、`eth_chainId` 分发、未知方法 `-32601`、参数超量 `-32602`、方法内 panic 被 recover 成 `-32603`。

**端到端(可选)**:`cast chain-id --rpc-url http://127.0.0.1:8669/rpc`、`cast block-number --rpc-url ...`(Foundry)。

---

## 8. 后续衔接点(bootstrap 已预留)

- **多 namespace / 同一 Server**:`New` 里已按 `RegisterName(ns, svc)` 组织,Phase 1 加方法只是"新 struct + 一行注册"。
- **同一 `/rpc` 路径挂 WS**:`Mount` 已标注第二条路由(`GET + Upgrade`)的插入点。
- **Backend 抽象**:`backend` 已隔离数据访问,Phase 1 扩接口即可。
- **错误模型**:`DataError` 接口就位,vechain 业务错误(过期 BlockRef、revert reason)Phase 1 直接实现它挂 `-32000` 系列 `data`。
- **revision 映射**:`eth_getBalance` 已标 TODO,Phase 1 补 `BlockNumberOrHash` union + ethereum↔thor 双向映射。
- **中间件错误信封一致性**(见 §2.5):Phase 1+ 把 `/rpc` 上的 413/403/500/timeout 出口包成 `-326xx` JSON-RPC 错误;加 WS 前先拆 §2.5 列的三个雷(全局 ReadTimeout、request-logger 与 timeout 的 WS 豁免)。
