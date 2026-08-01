# qdmp-server-sdk

[千岛小程序开放平台（qdmp）OpenAPI](https://open.qiandao.com/docs/api/auth-token) 官方 Server SDK，[Node.js](#nodejs)、[Java](#java)、[Go](#go) 三端实现，统一 token 生命周期管理 + 类型安全的业务接口封装。

## Node.js

```bash
npm install @qdmp/qdmp-server-sdk
```

```ts
import { QdmpClient, QdmpApiError, QdmpValidationError } from '@qdmp/qdmp-server-sdk'

const qdmp = new QdmpClient({
  appId: process.env.QDMP_APP_ID!,
  appSecret: process.env.QDMP_APP_SECRET!,
})

// 小程序端 wx.login() 拿到 code 后，后端用它换用户 session（不缓存，自己持久化）
const session = await qdmp.auth.code2Session(code)

// 用户级 accessToken 调业务接口，ctx.accessToken 必填
const me = await qdmp.user.me({ accessToken: session.accessToken })

try {
  await qdmp.mark.add({ accessToken: session.accessToken }, { spuId: '123', rating: { value: 5 } })
} catch (err) {
  if (err instanceof QdmpApiError) console.error(err.code, err.message, err.httpStatus)
  else if (err instanceof QdmpValidationError) console.error('本地参数错误：', err.message)
}

// app 级 token 只在需要时显式换取（例如可选的 app-token 兜底调用场景）
const appToken = await qdmp.auth.getAccessToken()
```

失败统一抛 `QdmpApiError`（业务失败）或 `QdmpValidationError`（本地参数校验失败，不发请求）。

## Java

```gradle
dependencies {
  implementation("io.github.echotechfe:qdmp-server-sdk:<version>")
}
```

```java
QdmpClient qdmp = new QdmpClient(
    QdmpClientConfig.builder()
        .appId(System.getenv("QDMP_APP_ID"))
        .appSecret(System.getenv("QDMP_APP_SECRET"))
        .build());

// 用 code 换用户 session（SDK 不缓存，自行持久化）
Code2SessionResult session = qdmp.auth().code2Session(code);

// QdmpContext.of(accessToken) 是唯一入口，空/非法 token 在构造期就失败
QdmpContext ctx = QdmpContext.of(session.getAccessToken());
UserMe200ResponseAllOfData me = qdmp.user().me(ctx);

qdmp.mark().add(ctx, new MarkAddRequest().spuId("123"));

// app 级 token 显式换取
String appToken = qdmp.auth().getAccessToken();
```

业务失败抛 `QdmpApiError`，传输层异常抛 `QdmpTransportException`，本地参数校验失败抛 `QdmpValidationError`（均在 `io.github.echotechfe.qdmp.errors` 包下）。

## Go

```bash
go get github.com/EchoTechFE/qdmp-server-sdk/go
```

```go
client, err := qdmp.NewClient(qdmp.ClientOptions{
    AppID:     os.Getenv("QDMP_APP_ID"),
    AppSecret: os.Getenv("QDMP_APP_SECRET"),
})

// 用 code 换用户 session（SDK 不缓存，自行持久化）
session, err := client.Auth.Code2Session(ctx, code)

// WithAccessToken 派生一个绑定了该 token 的子 client，拿不到 token 就构造不出可用的调用入口
asUser := client.WithAccessToken(session.AccessToken)
me, err := asUser.User.Me(ctx)
_, err = asUser.Mark.Add(ctx, generated.MarkAddJSONBody{SpuId: "123"})

// app 级 token 只在根 client 上，显式换取
appToken, err := client.Auth.GetAccessToken(ctx)
```

业务失败返回 `*qdmp.QdmpApiError`，缺 token 返回哨兵错误 `qdmp.ErrAccessTokenRequired`（`errors.Is` 判断），均是普通 `error`，不 panic。

## Token 生命周期模型

- **App 级 token（`CLIENT_CREDENTIALS`）**：SDK 自动缓存 + 到期前 300 秒刷新 + 单飞锁防并发重复换取，持久化走可插拔的 `TokenStore`（默认内存实现，可换 Redis 等共享存储）。调用方只需要 `auth.getAccessToken()`。
- **用户级 token（`AUTHORIZATION_CODE` 换来的）**：SDK 不缓存，每次调用由调用方显式传入——一个进程要服务海量终端用户，SDK 无法替调用方判断"这次调用是哪个用户"，这只有调用方自己的 session/DB 知道。
- SDK 不做业务请求的自动重试/401 恢复：用户级 token 由调用方持有，SDK 直接把 `QdmpApiError`（`httpStatus=401`）透传给调用方处理。
- 判断调用是否成功统一看响应体 `code === '0'`，不看 HTTP 状态码——实测 `refreshToken` 过期是 HTTP 200 + `code:10008`，access-token 失效是 HTTP 401 + `code:10005`。

## 各分组 token 要求

| 分组 | 要求 |
|---|---|
| `auth.token` / `auth.refresh` | 不需要 token（用 appId/appSecret） |
| `user.me` / `mark.*` / `wishspu.*` | 必须传用户级 accessToken，缺失本地直接报错，不发请求 |
| `island.*` / `spu.*` / `tag.*` / `genai.*` | 默认必须传用户级 accessToken；无证据表明 app 级 token 能安全调用，v1 不做默认 fallback |

上述所有分组都可以显式把 `auth.getAccessToken()` 换出的 app 级 token 当作 accessToken 传入，但 SDK 不会静默做这个 fallback。`x-echo-qdmp-version` 头只在 `standard` 认证方案下发送，`genai` 分组用另一对头，`auth` 分组不需要——三端都从 `shared/generated/route-meta.json` 读取每个 operation 的 `authScheme`/`tokenRequired`。

## 底层依赖

| 语言 | HTTP 传输 | JSON |
|---|---|---|
| Node.js | 内置全局 `fetch`（`undici` 仅作为开发依赖，提供测试用的 `MockAgent`） | 内置 `JSON` |
| Java | OkHttp（`okhttp3`） | Jackson |
| Go | 标准库 `net/http` | 标准库 `encoding/json` |

类型/DTO 由 OpenAPI 3.0 spec（`shared/openapi.yaml`）生成，三端分别用 `openapi-typescript`、`openapi-generator-cli`、`oapi-codegen`——只生成类型，不生成完整 client，业务方法和请求逻辑都是手写的。

## 目录结构

```
qdmp-server-sdk/
├─ shared/  # 单一真源 openapi.yaml + 生成的路由元数据/错误码表
├─ node/    # npm: @qdmp/qdmp-server-sdk
├─ java/    # Maven: io.github.echotechfe:qdmp-server-sdk
└─ go/      # module: github.com/EchoTechFE/qdmp-server-sdk/go
```

## 开发

本地环境搭建、代码生成、构建/测试/覆盖率命令见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## License

[MIT](./LICENSE)
