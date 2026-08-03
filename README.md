# qdmp-server-sdk

[千岛小程序开放平台 OpenAPI](https://open.qiandao.com/docs/api/auth-token) 官方 Server SDK，[Node.js](#nodejs)、[Java](#java)、[Go](#go) 三端实现，类型安全的业务接口封装 + 应用凭证自动缓存。

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

// 前端 qd.login() 拿到的一次性授权码传到服务端，换用户授权凭证
const credential = await qdmp.auth.getUserAccessToken(code)
// => { accessToken, refreshToken, expiresAt, openId }，自己按 openId 存起来

// 调业务接口：accessToken 每次显式传进去
const me = await qdmp.user.me({ accessToken: credential.accessToken })
await qdmp.mark.add({ accessToken: credential.accessToken }, { spuId: '123', rating: { value: 5 } })

// accessToken 过期了，自己拿 refreshToken 换新的（SDK 不代管，也不自动重试）
const fresh = await qdmp.auth.refreshToken(credential.refreshToken)
await qdmp.user.me({ accessToken: fresh.accessToken })

try {
  await qdmp.wishspu.list({ accessToken: fresh.accessToken }, { offset: '0', limit: '20' })
} catch (err) {
  if (err instanceof QdmpApiError) console.error(err.code, err.message, err.httpStatus)
  else if (err instanceof QdmpValidationError) console.error('本地参数错误：', err.message)
}

// 应用凭证，开发调试/后台任务才需要
const appCredential = await qdmp.auth.getAppAccessToken()
// => { accessToken, expiresAt, refreshToken, openId }（openId 恒为空串）
```

失败抛三类错误：`QdmpApiError`（业务失败，响应体 `code` 非 `'0'`）、`QdmpValidationError`（本地参数校验失败，不发请求）、`QdmpTransportError`（传输层问题，如收到重定向、响应体超过 10MB）。底层 `fetch` 本身的网络错误原样透出。

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

UserAccessTokenResult credential = qdmp.auth().getUserAccessToken(code);

// QdmpContext.of(accessToken) 是唯一入口，空/非法 token 在构造期就失败
QdmpContext ctx = QdmpContext.of(credential.getAccessToken());
UserMe200ResponseAllOfData me = qdmp.user().me(ctx);
qdmp.mark().add(ctx, new MarkAddRequest().spuId("123"));

// 过期了自己换，SDK 不代管
RefreshTokenResult fresh = qdmp.auth().refreshToken(credential.getRefreshToken());
qdmp.user().me(QdmpContext.of(fresh.getAccessToken()));

AppAccessTokenResult appCredential = qdmp.auth().getAppAccessToken();
```

业务失败抛 `QdmpApiError`，传输层异常抛 `QdmpTransportException`，`auth.*` 的参数校验失败抛 `QdmpValidationError`（均在 `io.github.echotechfe.qdmp.errors` 包下）。`QdmpContext.of()` 的 token 校验按 Java 惯例抛 `NullPointerException`（null）/ `IllegalArgumentException`（空白或含不能进 HTTP 头的字符）。

## Go

```bash
go get github.com/EchoTechFE/qdmp-server-sdk/go
```

```go
client, err := qdmp.NewClient(qdmp.ClientOptions{
    AppID:     os.Getenv("QDMP_APP_ID"),
    AppSecret: os.Getenv("QDMP_APP_SECRET"),
})

credential, err := client.Auth.GetUserAccessToken(ctx, code)

// 调业务接口：accessToken 每次显式传进去（ctx 仍是标准的 context.Context，管超时和取消）
qdmpCtx := qdmp.Context{AccessToken: credential.AccessToken}
me, err := client.User.Me(ctx, qdmpCtx)
_, err = client.Mark.Add(ctx, qdmpCtx, generated.MarkAddJSONBody{SpuId: "123"})

// 过期了自己换，SDK 不代管
fresh, err := client.Auth.RefreshToken(ctx, credential.RefreshToken)
me, err = client.User.Me(ctx, qdmp.Context{AccessToken: fresh.AccessToken})

appCredential, err := client.Auth.GetAppAccessToken(ctx)
```

业务失败返回 `*qdmp.QdmpApiError`，缺凭证返回哨兵错误 `qdmp.ErrAccessTokenRequired`（`errors.Is` 判断），均是普通 `error`，不 panic。

## 凭证生命周期模型

- **用户授权凭证由调用方自己管**：SDK 不缓存、不代管、不自动续期。每次业务调用把 accessToken 显式传进去。
  一个进程要服务海量终端用户，哪次调用属于哪个用户、凭证该存到哪、什么时候该续期，只有你自己知道；
  你也可以完全不用 `getUserAccessToken`，自己实现拿凭证那一步，SDK 照样能用。
- **续期**：`auth.refreshToken(refreshToken)` 换一个新的 accessToken，一次性调用，SDK 不缓存结果。
  `/auth/v1/refresh` 只返回新的 accessToken 和 expiresAt，**refreshToken 保持不变**。
  它本身失效时返回 HTTP 200 + `10007`/`10008`，此时凭证已无法挽救，需要重新走一次「拿授权码 → 换凭证」。
- **SDK 不做任何自动重试**：业务调用撞上 HTTP 401 + `10005`/`10006`（access_token 失效/过期）时，
  直接把 `QdmpApiError` 抛/返回给你，由你决定是否续期后重试。
- **应用凭证**：`getAppAccessToken()` 自动缓存 + 到期前 300 秒重新换取 + 单飞锁防并发重复换取，
  持久化走可插拔的 `TokenStore`（默认内存实现，多实例部署可换 Redis 等共享存储）。
  它返回的是完整凭证 `{accessToken, expiresAt, refreshToken, openId}`——应用凭证响应里同样带 refreshToken，
  只是 `openId` 恒为空串。SDK 自己不拿它续期（到期直接重换），但如实交给你，要不要自己续期由你决定。
- **成功判定只看响应体 `code === '0'`，不看 HTTP 状态码**——refreshToken 过期是 HTTP 200 + `10008`，
  access_token 失效才是 HTTP 401 + `10006`。

## 各分组凭证要求

| 分组 | 要求 |
|---|---|
| `auth.*` | 不需要凭证（用 appId/appSecret 或 refreshToken） |
| `user.me` / `mark.*` / `wishspu.*` | 必须是用户授权凭证，缺失时本地直接报错，不发请求 |
| `island.*` / `spu.*` / `tag.*` / `genai.*` | 应用凭证也实测调通过，但 SDK 不做静默 fallback，传哪种由调用方决定 |

`x-echo-qdmp-version` 头只在 `standard` 鉴权方案下发送，`genai` 分组用 `x-openapi-access-token` + `x-openapi-app-id` 另一对头，`auth` 分组不需要——三端都从 `shared/generated/route-meta.json` 读取每个 operation 的 `authScheme`/`tokenRequired`。

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
