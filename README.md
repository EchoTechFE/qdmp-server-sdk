# qdmp-server-sdk

[![npm](https://img.shields.io/npm/v/@qdmp/qdmp-server-sdk?label=npm)](https://www.npmjs.com/package/@qdmp/qdmp-server-sdk)
[![Go Reference](https://pkg.go.dev/badge/github.com/EchoTechFE/qdmp-server-sdk/go.svg)](https://pkg.go.dev/github.com/EchoTechFE/qdmp-server-sdk/go)
[![Node CI](https://github.com/EchoTechFE/qdmp-server-sdk/actions/workflows/node-ci.yml/badge.svg?branch=main)](https://github.com/EchoTechFE/qdmp-server-sdk/actions/workflows/node-ci.yml)
[![Java CI](https://github.com/EchoTechFE/qdmp-server-sdk/actions/workflows/java-ci.yml/badge.svg?branch=main)](https://github.com/EchoTechFE/qdmp-server-sdk/actions/workflows/java-ci.yml)
[![Go CI](https://github.com/EchoTechFE/qdmp-server-sdk/actions/workflows/go-ci.yml/badge.svg?branch=main)](https://github.com/EchoTechFE/qdmp-server-sdk/actions/workflows/go-ci.yml)
[![License](https://img.shields.io/github/license/EchoTechFE/qdmp-server-sdk)](./LICENSE)

[千岛小程序开放平台 OpenAPI](https://open.qiandao.com/docs/api) 官方服务端 SDK，提供 Node.js、Go 和 Java 实现。三端共用一份接口定义，请求参数和响应结果都有对应类型。

## SDK 状态

| 语言 | 环境要求 | 获取方式 |
| --- | --- | --- |
| Node.js | Node.js 22+ | [npm](https://www.npmjs.com/package/@qdmp/qdmp-server-sdk) |
| Go | Go 1.24+ | [Go Reference](https://pkg.go.dev/github.com/EchoTechFE/qdmp-server-sdk/go) |
| Java | Java 11+ | 暂未发布到 Maven Central，可从 [`java/`](./java/) 构建 |

## Node.js 快速开始

安装：

```bash
npm install @qdmp/qdmp-server-sdk
```

初始化客户端后，用前端 `qd.login()` 返回的一次性授权码换取用户凭证：

```ts
import { QdmpClient } from '@qdmp/qdmp-server-sdk'

const qdmp = new QdmpClient({
  appId: process.env.QDMP_APP_ID!,
  appSecret: process.env.QDMP_APP_SECRET!,
})

const credential = await qdmp.auth.getUserAccessToken(code)
const context = { accessToken: credential.accessToken }

const me = await qdmp.user.me(context)
console.log(me)
await qdmp.mark.add(context, {
  spuId: '123',
  rating: { value: 5 },
})
```

access token 过期后，由业务代码发起续期并保存新结果：

```ts
const fresh = await qdmp.auth.refreshToken(credential.refreshToken)
await qdmp.user.me({ accessToken: fresh.accessToken })
```

## Go 快速开始

安装：

```bash
go get github.com/EchoTechFE/qdmp-server-sdk/go
```

下面的代码放在已有 `context.Context` 和授权码 `code` 的处理函数中：

```go
import (
	"os"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

client, err := qdmp.NewClient(qdmp.ClientOptions{
	AppID:     os.Getenv("QDMP_APP_ID"),
	AppSecret: os.Getenv("QDMP_APP_SECRET"),
})
if err != nil {
	return err
}

credential, err := client.Auth.GetUserAccessToken(ctx, code)
if err != nil {
	return err
}

requestContext := qdmp.Context{AccessToken: credential.AccessToken}
_, err = client.User.Me(ctx, requestContext)
if err != nil {
	return err
}

_, err = client.Mark.Add(ctx, requestContext, generated.MarkAddJSONBody{SpuId: "123"})
if err != nil {
	return err
}

return nil
```

续期时调用 `client.Auth.RefreshToken(ctx, credential.RefreshToken)`，然后保存并使用返回的新 access token。

## Java

Java SDK 暂未发布到 Maven Central。当前版本可在仓库中构建：

```bash
cd java
./gradlew build
```

API 用法如下：

```java
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpClientConfig;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.generated.MarkAddRequest;

QdmpClient qdmp = new QdmpClient(
    QdmpClientConfig.builder()
        .appId(System.getenv("QDMP_APP_ID"))
        .appSecret(System.getenv("QDMP_APP_SECRET"))
        .build());

var credential = qdmp.auth().getUserAccessToken(code);
var context = QdmpContext.of(credential.getAccessToken());

var me = qdmp.user().me(context);
qdmp.mark().add(context, new MarkAddRequest().spuId("123"));
```

续期时调用 `qdmp.auth().refreshToken(credential.getRefreshToken())`，然后用返回的新 access token 创建 `QdmpContext`。

## 凭证怎么用

平台有两种凭证，SDK 对它们的处理方式不同。

### 用户授权凭证

用户授权凭证代表当前登录用户。服务端拿到前端 `qd.login()` 返回的授权码后，调用 `getUserAccessToken` 换取 access token、refresh token 和过期时间。返回结果可以按 `openId` 保存。

SDK 不保存或自动续期用户凭证。每次业务调用都要显式传入 access token；过期后调用 `refreshToken`，由业务代码保存新的 access token。该接口不会更换 refresh token。

SDK 也不会自动重试失败的业务请求。遇到 HTTP 401 和错误码 `10005`、`10006` 时，是否续期并重试由业务代码决定。续期接口返回错误码 `10007`、`10008` 时，需要让用户重新授权。

### 应用凭证

应用凭证代表应用本身，适合开发调试、服务端联调和不需要用户身份的后台任务。以 Node.js 为例：

```ts
const credential = await qdmp.auth.getAppAccessToken()
```

`getAppAccessToken` 会缓存凭证，并在到期前 300 秒重新获取。默认缓存在当前进程；多实例部署可以实现 `TokenStore`，改用 Redis 等共享存储。

## 接口需要哪种凭证

| 接口分组 | 凭证要求 |
| --- | --- |
| `auth.*` | 不需要额外凭证，调用时使用 appId、appSecret 或 refresh token |
| `user.me`、`mark.*`、`wishspu.*`、`post.*`、`comment.*` | 必须传用户授权凭证；缺少凭证时不会发出请求 |
| `island.*`、`spu.*`、`tag.*`、`genai.*` | 显式传入 access token；应用凭证可用 |

接口是否成功以响应体的 `code === '0'` 为准，不只看 HTTP 状态码。

## 错误处理

- Node.js：业务错误为 `QdmpApiError`，本地参数错误为 `QdmpValidationError`，重定向或响应体过大等传输错误为 `QdmpTransportError`。底层 `fetch` 的网络错误会原样抛出。
- Java：业务错误为 `QdmpApiError`，参数错误为 `QdmpValidationError`，传输错误为 `QdmpTransportException`，都在 `io.github.echotechfe.qdmp.errors` 包中。
- Go：业务错误为 `*qdmp.QdmpApiError`；缺少凭证时返回 `qdmp.ErrAccessTokenRequired`，可用 `errors.Is` 判断。SDK 返回普通 `error`，不会 panic。

## 开发

`shared/openapi.yaml` 是三端共用的接口定义。代码生成、构建、测试和覆盖率命令见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## License

[MIT](./LICENSE)
