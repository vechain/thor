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

### 1.Get account by address

```
URL: /address/{address}
Method : GET
```

| Params  | type   | required |
| ------- | ------ | -------- |
| address | string | Yes      |

```
{
  "Balance":0,
  "CodeHash":cry.Hash
  "StorageRoot":cry.Hash
}
```

---





