# 37 crypto 加密与哈希

> Go 版本：>= 1.27.1  
> 相关标准库：crypto/sha256、crypto/sha3、crypto/mldsa、crypto/ecdh、crypto/mlkem、crypto/hpke

本篇讲标准库的密码学原语：内容哈希（`crypto/sha256`、`crypto/sha3`）、数字签名（`crypto/mldsa`）、密钥交换与混合加密（`crypto/ecdh`、`crypto/mlkem`、`crypto/hpke`）。前置只有一处：随机数来自 `crypto/rand`（见 `25-数学与随机数.md`）；后量子签名与混合加密属于"用到再看"的内容，篇内已标出可跳过。

## crypto

### crypto/sha256

计算 SHA-256：

```go
sum := sha256.Sum256([]byte("hello"))

fmt.Printf("%x", sum) // 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
```

流式计算：

```go
h := sha256.New()

h.Write([]byte("hello"))
h.Write([]byte("world"))

fmt.Printf("%x", h.Sum(nil)) // 936a185caaa266bb9cbe981e9e05cb78cd732b0b3280eb944412bb6f8f8f07af —— 与一次性写入 "helloworld" 等价
```

SHA-256 用于完整性校验与内容寻址。它**不是**密码哈希：存密码要用专门的口令哈希算法，或者标准库自 Go 1.24 起提供的 `crypto/pbkdf2`（`pbkdf2.Key`，RFC 8018 的 PBKDF2）、`crypto/hkdf`（RFC 5869 的 HKDF）；bcrypt、scrypt、argon2 仍在 `golang.org/x/crypto` 里，属于第三方，本系列不展开。自己用 SHA-256 加盐存密码是不合格的。

安全随机数（密钥、token、验证码）见 `25-数学与随机数.md`。

### crypto/sha3（Go 1.24 新增）

SHA-3 家族（FIPS 202）在 Go 1.24 进入标准库。用法和 `crypto/sha256` 一样简单：

```go
fmt.Printf("%x\n", sha3.Sum256([]byte("hello")))
// 3338be694f50c5f338814986cdf0686453a888b84f424d792af4b9202398f392

fmt.Println(len(sha3.Sum512(nil))) // 64
```

- `sha3.Sum224` / `Sum256` / `Sum384` / `Sum512` 是四个定长版本；流式写法是 `sha3.New256()` 等，和 `sha256.New()` 同构。
- **SHA-3 和 SHA-2 不是"新的取代旧的"关系**，两者并存：SHA-2 依然是绝大多数场景（TLS、签名、内容寻址）的默认选择，SHA-3 用于需要不同结构假设、或规范明确要求的场合。
- 和 SHA-256 一样，**它也只是哈希，不是密码哈希**——存口令仍然要用 `crypto/pbkdf2` / `crypto/hkdf`（见上文）。

### crypto/mldsa（Go 1.27 新增）

**这一节讲后量子签名，日常项目基本用不到，初读可以跳过**——真要做抗量子的签名或密钥交换时再回来。

Go 1.27 把后量子签名算法 ML-DSA（即 FIPS 204）加进了标准库，用法与 `crypto/ecdsa` 类似：

```go
msg := []byte("hello")

priv, err := mldsa.GenerateKey(mldsa.MLDSA65())
if err != nil {
    return err
}

// Sign 返回两个值（签名、错误）；第一个参数是 io.Reader，被忽略，直接传 nil
sig, err := priv.Sign(nil, msg, nil)
if err != nil {
    return err
}

err = mldsa.Verify(priv.PublicKey(), msg, sig, nil) // 验签通过返回 nil
fmt.Println(err, len(sig))                          // <nil> 3309，等于该档位的 SignatureSize()
```

三个档位 `MLDSA44` / `MLDSA65` / `MLDSA87` 对应不同强度——包文档说**多数应用应该用 `MLDSA44`**，65/87 是给更高安全等级用的，代价是密钥和签名都很大（65 档签名 3309 字节、公钥 1952 字节，远大于 RSA/ECDSA，协议设计时要留出空间）。具体大小可以从参数上取，不要写死：

```go
params := mldsa.MLDSA65()
fmt.Println(params.SignatureSize(), params.PublicKeySize()) // 3309 1952
```

验签失败时返回的错误不只有一种（还有 `mldsa: invalid signature length`、`mldsa: invalid signature encoding`），所以判 `err != nil` 就够了，不要去匹配错误文本。如果需要签名上下文，`Options.Context` 在签名和验签两侧必须一致。同一版本也给 `crypto/tls` 和 `crypto/x509` 加了对应的算法标识。

> **关于稳定性**：ML-DSA 本身是 FIPS 204 定稿标准，算法不会变；但**这个包的 Go API 还很年轻**（Go 1.27 才进入标准库），档位选择和 `Options` 的字段在后续版本仍可能调整。另外它**依赖加密模块版本**：官方文档明确说在 FIPS 140-3 Go Cryptographic Module v1.0.0 下这个包不可用（`GenerateKey` 等会返回错误），需要 v1.26.0 以上。生产环境引入前先确认自己的合规要求。

### 密钥交换与混合加密：crypto/ecdh、crypto/mlkem、crypto/hpke

这三者解决的是"双方怎么安全地协商出同一把密钥"。

**`crypto/ecdh`**（Go 1.20 起）是椭圆曲线 Diffie-Hellman 的标准入口，比老的 `crypto/elliptic` 底层接口更不容易用错：

```go
curve := ecdh.P256()

priv, err := curve.GenerateKey(rand.Reader) // rand 见 25-数学与随机数.md 的 crypto/rand
if err != nil {
    return err
}

// 把自己的公钥发出去，用对方的公钥算出共享密钥
shared, err := priv.ECDH(peerPublicKey)
```

要点：两边用**各自的私钥 + 对方的公钥**算出的 `shared` 完全相同，它就是后续对称加密的密钥；P-256 的公钥是 65 字节。`ecdh` 只做密钥协商，**不负责认证对方身份**——防中间人要靠 TLS 的证书链，或者自己对公钥做签名校验。

**`crypto/mlkem`**（Go 1.24 起）是后量子密钥封装（ML-KEM，即 FIPS 203）。它是"KEM"而不是 DH：一方**封装**出共享密钥和一段密文，另一方用私钥**解封装**：

```go
dk, err := mlkem.GenerateKey768()
if err != nil {
    return err
}

// 发送方：得到共享密钥、以及要传给接收方的密文
shared, ciphertext := dk.EncapsulationKey().Encapsulate()

// 接收方：用自己的私钥从密文里恢复出同一个共享密钥
shared2, err := dk.Decapsulate(ciphertext)

fmt.Println(string(shared) == string(shared2), err == nil) // true true
```

只有 **768 和 1024 两个档位**（`GenerateKey768` / `GenerateKey1024`，各自有独立的类型）。抗量子的代价是体积：ML-KEM-768 的公钥就有 **1184 字节**，远大于 P-256 的 65 字节，协议设计时要留出空间。

**`crypto/hpke`**（Go 1.26 起）把上面这些组合成一套完整的"混合公钥加密"（RFC 9180）：选好 KEM + KDF + AEAD，用一个 `Seal` / `Open` 就对"用接收方公钥加密一段载荷"这件事一步到位：

```go
kem := hpke.DHKEM(ecdh.X25519())

// 接收方生成密钥对，公钥发出去
sk, err := kem.GenerateKey()
if err != nil {
    return err
}
pk := sk.PublicKey()

// 发送方：用接收方公钥密封载荷（info 是双方约定的上下文，两侧必须一致）
ct, err := hpke.Seal(pk, hpke.HKDFSHA256(), hpke.AES128GCM(), []byte("my-app"), payload)
if err != nil {
    return err
}

// 接收方：用自己的私钥打开
pt, err := hpke.Open(sk, hpke.HKDFSHA256(), hpke.AES128GCM(), []byte("my-app"), ct)
fmt.Println(string(pt), err) // 原来的载荷 <nil>
```

`Seal` 返回的是"封装的密钥 + 密文"的拼接，一次调用就把密钥协商和加密都做完了。KEM 可以换：`hpke.DHKEM(ecdh.P256())` 是经典方案，`hpke.MLKEM768X25519()`（即 X-Wing）是"抗量子 + 经典"的混合方案，这正是"既想要后量子安全、又不想完全押注新算法"时该选的。

选型建议：

- 走 TLS / HTTP / SSH 时**不需要直接碰这些包**，协议栈已经选好了（TLS 自己也在做混合密钥交换）。
- 要自己设计密钥协商、或者做端到端加密与密封信封时：经典用 `ecdh`，抗量子用 `mlkem`，想一步到位就用 `hpke`。
- 无论选哪个，**身份认证都是另一件事**：这些 API 保证的是"只有持有私钥的一方能解开"，不保证"公钥真的属于你以为的那个人"。

密码学用途要和普通数据结构的用途分清：内容校验、去重、缓存 key 用 `map` 或 `hash/maphash` 就够，用不到 `crypto` 下的包；反过来，凡是涉及防伪造、防猜测的地方都必须用标准库的密码学原语，不要自己拼一个。

下一篇进入 [`38-并发基础.md`](38-并发基础.md)。
