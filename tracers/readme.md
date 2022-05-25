based on github.com/ethereum/go-ethereum/eth/tracers v1.8.14 tag

2020-10-16(rebase to v1.9.23)

+ merge commit https://github.com/ethereum/go-ethereum/commit/e8ff318205be2d3e9f793ee876726bf0fbaf579e (eth/tracer: extend create2)
+ merge commit https://github.com/ethereum/go-ethereum/commit/dfa16a3e4e0e0b5b20bfda7b7e89ebd07ea0a1a5 (eth/tracers: fixed incorrect storage from prestate_tracer)
+ merge commit https://github.com/ethereum/go-ethereum/commit/71c37d82adaa2b69ea98ce0c5505489d6b711c1e (js/tracers: make call tracer report value in selfdestructs)
+ merge commit https://github.com/ethereum/go-ethereum/commit/05280a7ae3f47adc8aeb9130c7f5404a42fb3a55 (eth/tracers: revert reason in call_tracer + error for failed internal calls)

2022-05-07

introduce native tracers, forked from https://github.com/ethereum/go-ethereum/tree/v1.10.18 (de23cf910b814867d5c5d1ad6164835d79069638)

+ updated vm implementation, added CaptureEnter and CaptureExit
+ remove CaptureTxStart CaptureTxEnd from Logger interface
+ update contract create function in `prestate`
+ remove js tracers