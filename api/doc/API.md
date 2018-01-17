[TOC]

---

## Http status code

```
200 OK
400 Bad request
500 server error
```

---

## 1.Account

---

```
Pathprefix /account
```

---

### 1. Get balance by address

```
URL: /address/{address}/balance
Method : GET
```

| Params  | Type   | Required | Remark     |
| ------- | ------ | -------- | ---------- |
| address | string | Yes      | an address |

```
Description:
	return balance of `address`,return 0 if `address` does not exist.
```

```
Format:JSON
-----------
Example:
{
  "balance":0,
}
```

---

### 2. Get code by address

```
URL: /address/{address}/code
Method : GET
```

| Params  | Type   | Required | Remark     |
| ------- | ------ | -------- | ---------- |
| address | string | Yes      | an address |

```
Description:
	return code of `address`,return nil if `address` does not exist.
```

```
Format:JSON
-----------
Example:
{
  "code":[0x00],
}
```

------

### 3. Get storage by address and storage key

```
URL: /address/{address}/storage
Method : GET
```

| Params  | Type   | Required | Remark                          |
| ------- | ------ | -------- | ------------------------------- |
| address | string | Yes      | an address                      |
| key     | string | Yes      | key of storage, which is a hash |

```
Description:
	return storage within `address` and `key`, return nil if `address` does not exist.
```

```
Format:JSON
-----------
Example:
{
  "0x00000000000000000000000000000000000000000000000000000000006b6579":
  "0x0000000000000000000000000000000000000000000000000000000000007631",
}
```

------

## 2. Transaction

------

```
Pathprefix /transaction
```

------

### 1. Get a transaction by transaction hash

```
URL: /hash/{hash}
Method : GET
```

| Params | Type   | Required | Remark                |
| ------ | ------ | -------- | --------------------- |
| hash   | string | Yes      | a hash of transaction |

```
Description:
	return a transaction by `hash`,return nil if transaction does not exist.
```

```
Format:JSON
-----------
Example:
{
"Hash":"0x5b593c087a61dfff29d5c908aca972c64d0b1242340170817cbafb59d568692f",
"GasPrice":1000,
"Gas":1000,
"TimeBarrier":10000,
"From":"0x5736328b5309f288872c45924bfafeef5dc39352",
"Clauses":[
			{
			  "To":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01a",
			  "Value":10,
			  "Data":[0x00]
			 }
			]
}
```

------

### 2. Get a transaction with block number and transaction index

```
URL: /blocknumber/{number}/txindex/{index}
Method : GET
```

| Params | Type   | Required | Remark                     |
| ------ | ------ | -------- | -------------------------- |
| number | number | Yes      | block number               |
| index  | number | Yes      | transaction index (from 0) |

```
Description:
	return a transaction by `number` and `index`,return nil if transaction does not exist.
```

```
Format:JSON
-----------
Example:
{
"Hash":"0x5b593c087a61dfff29d5c908aca972c64d0b1242340170817cbafb59d568692f",
"GasPrice":1000,
"Gas":1000,
"TimeBarrier":10000,
"From":"0x5736328b5309f288872c45924bfafeef5dc39352",
"Clauses":[
			{
			  "To":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01a",
			  "Value":10,
			  "Data":[0x00]
			 }
			]
}
```

------

## 3. Block

------

```
Pathprefix /block
```

------

### 1. Get a block by block hash

```
URL: /hash/{hash}
Method : GET
```

| Params | Type   | Required | Remark     |
| ------ | ------ | -------- | ---------- |
| hash   | string | Yes      | block hash |

```
Description:
	return a block by `hash`,return nil if block does not exist.
```

```
Format:JSON
-----------
Example:

{
 "Number":1,
 "Hash":"0x0000000176e487712d247d26b5e756d9692dd34d0113c00d063f202d740a85b9",
 "ParentHash":
 "0x000000006d2958e8510b1503f612894e9223936f1008be2a218c310fa8c114      23",
 "Timestamp":0,
 "TotalScore":0,
 "GasLimit":0,
 "GasUsed":0,
 "Beneficiary":"0x0000000000000000000000000000000000000000", 
 "TxsRoot":
 "0xf16a30319a81397485d1e647d5fb3ebed298f6be298485850c892116438ad5ca",
 "StateRoot":
 "0x0000000000000000000000000000000000000000000000000000000000000000",
 "ReceiptsRoot":
 "0x0000000000000000000000000000000000000000000000000000000000000000",
 "Txs":
     	[
 		"0x2f254869e098aeb1b41c98427fb78322c1d85ee23a77f81caab7172755d436ff"
 		]
 }
```

------

### 2. Get a block by block number

```
URL: /number/{number}
Method : GET
```

| Params | Type   | Required | Remark       |
| ------ | ------ | -------- | ------------ |
| number | number | Yes      | block number |

```
Description:
	return a block by `number`,return nil if block does not exist.
```

```
Format:JSON
-----------
Example:

{
 "Number":1,
 "Hash":"0x0000000176e487712d247d26b5e756d9692dd34d0113c00d063f202d740a85b9",
 "ParentHash":
 "0x000000006d2958e8510b1503f612894e9223936f1008be2a218c310fa8c114      23",
 "Timestamp":0,
 "TotalScore":0,
 "GasLimit":0,
 "GasUsed":0,
 "Beneficiary":"0x0000000000000000000000000000000000000000", 
 "TxsRoot":
 "0xf16a30319a81397485d1e647d5fb3ebed298f6be298485850c892116438ad5ca",
 "StateRoot":
 "0x0000000000000000000000000000000000000000000000000000000000000000",
 "ReceiptsRoot":
 "0x0000000000000000000000000000000000000000000000000000000000000000",
 "Txs":
     	[
 		"0x2f254869e098aeb1b41c98427fb78322c1d85ee23a77f81caab7172755d436ff"
 		]
 }
```

------

### 

