# qdmp-server-sdk

[千岛小程序开放平台 OpenAPI](https://open.qiandao.com/docs/api/auth-token) 官方 Server SDK，[Node.js](#nodejs)、[Java](#java)、[Go](#go) 三端实现，统一凭证生命周期管理 + 类型安全的业务接口封装。

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

// 绑定凭证，之后业务调用不用再传 token；到期前自动续期
const asUser = qdmp.withUserCredential({
  ...credential,
  onRefresh: token => {
    // 续期后拿到新的 token.accessToken / token.expiresAt，存回你放凭证的地方
  },
})

const me = await asUser.user.me()
await asUser.mark.add({ spuId: '123', rating: { value: 5 } })

try {
  await asUser.wishspu.list({ offset: '0', limit: '20' })
} catch (err) {
  if (err instanceof QdmpApiError) console.error(err.code, err.message, err.httpStatus)
  else if (err instanceof QdmpValidationError) console.error('本地参数错误：', err.message)
}

// 应用凭证，开发调试/后台任务才需要
const appCredential = await qdmp.auth.getAppAccessToken()
// => { accessToken, expiresAt, refreshToken, openId }（openId 恒为空串）
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

UserAccessTokenResult credential = qdmp.auth().getUserAccessToken(code);

QdmpUserClient asUser = qdmp.withUserCredential(
    UserCredentialOptions.builder()
        .accessToken(credential.getAccessToken())
        .refreshToken(credential.getRefreshToken())
        .expiresAt(credential.getExpiresAtEpochSeconds())
        .onRefresh(token -> {
          // 续期后把新的 accessToken / expiresAt 存回你放凭证的地方
        })
        .build());

UserMe200ResponseAllOfData me = asUser.user().me();
asUser.mark().add(new MarkAddRequest().spuId("123"));

AppAccessTokenResult appCredential = qdmp.auth().getAppAccessToken();
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

credential, err := client.Auth.GetUserAccessToken(ctx, code)

asUser := client.WithUserCredential(qdmp.UserCredentialOptions{
    AccessToken:  credential.AccessToken,
    RefreshToken: credential.RefreshToken,
    ExpiresAt:    credential.ExpiresAt,
    OnRefresh: func(ctx context.Context, t qdmp.RefreshedUserToken) error {
        // 续期后把新的 t.AccessToken / t.ExpiresAt 存回你放凭证的地方
        return nil
    },
})

me, err := asUser.User.Me(ctx)
_, err = asUser.Mark.Add(ctx, generated.MarkAddJSONBody{SpuId: "123"})

appCredential, err := client.Auth.GetAppAccessToken(ctx)
```

业务失败返回 `*qdmp.QdmpApiError`，缺凭证返回哨兵错误 `qdmp.ErrAccessTokenRequired`（`errors.Is` 判断），均是普通 `error`，不 panic。

## 凭证生命周期模型

- **用户授权凭证**（主线）：`withUserCredential` 绑定一份凭证后，SDK 到期前 300 秒自动用 refreshToken 续期；
  业务调用撞上 HTTP 401 + `10005`/`10006`（access_token 失效/过期）时，续期一次并重试该请求一次。
  续期成功后回调 `onRefresh` 把新的 accessToken 交给你——SDK 不替你持久化：一个进程要服务海量终端用户，
  哪次调用属于哪个用户、凭证该存到哪，只有你自己知道。同一份凭证上的并发调用只会触发一次续期。
- **续期不换 refreshToken**：`/auth/v1/refresh` 只返回新的 accessToken 和 expiresAt，refreshToken 保持不变。
  它本身失效时返回 HTTP 200 + `10007`/`10008`，此时凭证已无法挽救，需要重新走一次「拿授权码 → 换凭证」。
- **应用凭证**：`getAppAccessToken()` 自动缓存 + 到期前 300 秒重新换取 + 单飞锁防并发重复换取，
  持久化走可插拔的 `TokenStore`（默认内存实现，多实例部署可换 Redis 等共享存储）。
  它返回的是完整凭证 `{accessToken, expiresAt, refreshToken, openId}`——应用凭证响应里同样带 refreshToken，
  只是 `openId` 恒为空串。SDK 自己不拿它续期（到期直接重换），但如实交给你，要不要自己续期由你决定。
- **成功判定只看响应体 `code === '0'`，不看 HTTP 状态码**——refreshToken 过期是 HTTP 200 + `10008`，
  access_token 失效才是 HTTP 401 + `10006`。
- 除上面那次 401 续期重试外，SDK 不做任何自动重试。

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
