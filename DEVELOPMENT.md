# 本地开发

面向本仓库贡献者的本地环境搭建、代码生成、构建/测试/覆盖率说明。用户接入文档见 [README.md](./README.md)。

## 环境依赖

- Node.js ≥ 22
- JDK ≥ 17（CI 用 temurin 17）
- Go（版本见 `go/go.mod`）
- [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen)：`go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest`，装完确保 `$(go env GOPATH)/bin` 在 `PATH` 上
- `openapi-generator-cli`（Java model 生成用）：`shared/scripts/regenerate-all.sh` 会在 `OPENAPI_GENERATOR_CLI_JAR` 未指向已存在的 jar 时自动下载到 `/tmp/tools/openapi-generator-cli.jar`，无需手动安装

## 代码生成

`shared/openapi.yaml` 是三端类型定义的单一真源。修改它之后必须重新生成：

```bash
bash shared/scripts/regenerate-all.sh
```

这一步会依次跑：
1. `shared/scripts/gen-route-meta.mjs`——解析 `x-qdmp-token-required`/`x-qdmp-auth-scheme`/`x-error-codes` 等自定义扩展字段（三个官方 codegen 工具都不认它们），生成 `shared/generated/{route-meta,error-codes}.json`。
2. Node：`openapi-typescript` 生成 `node/src/generated/schema.d.ts`，再跑 `node/scripts/gen-route-meta.mjs` 同步路由元数据/错误码到 TS。
3. Java：`openapi-generator-cli`（`--library=resttemplate`，models-only）生成 `java/generated/`，再跑 `java/scripts/gen-route-meta.mjs` 同步。
4. Go：`oapi-codegen` 生成 `go/generated/types.gen.go`。

CI 的 `generate-check.yml` 会重新跑一遍这个脚本并 `git diff --exit-code`，检测 spec 和已提交生成产物是否漂移——本地改完 spec 记得先跑这个脚本再提交。

## 构建 / Lint

```bash
# Node
cd node && npm ci && npm run typecheck && npm run lint && npm run build

# Java
cd java && ./gradlew checkstyleMain checkstyleTest spotlessCheck

# Go
cd go && gofmt -l . && go vet ./... && staticcheck ./...
```

三端都遵循 Google 官方风格指南，具体文档地址：

- **TypeScript**：[Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)，由 [`gts`](https://github.com/google/gts) 落地（`npm run lint` 检查 / `npm run fix` 自动修复）。
- **Java**：[Google Java Style Guide](https://google.github.io/styleguide/javaguide.html)，由 Checkstyle 的 `google_checks.xml`（随 checkstyle 工具本体分发）做静态检查，`google-java-format` 通过 Spotless 插件（`./gradlew spotlessApply` 自动修复）落地排版规则。
- **Go**：[Google Go Style Guide](https://google.github.io/styleguide/go/)（含 [Style Guide](https://google.github.io/styleguide/go/guide) / [Style Decisions](https://google.github.io/styleguide/go/decisions) / [Best Practices](https://google.github.io/styleguide/go/best-practices) 三部分），目前落地手段是 `gofmt` + `go vet` + `staticcheck`；仓库暂未接入面向 Google Go Style 的专用 lint 配置（如 golangci-lint），差异需要人工对照上述文档走查。

三端提交前必须跑通对应语言的 lint/format 检查（见上一节命令），不允许绕过。

## 测试

```bash
# Node
cd node && npm test

# Java
cd java && ./gradlew test

# Go
cd go && go test ./...
```

三端测试都是 mock HTTP（Node 用 undici `MockAgent`，Java 用 `MockWebServer`，Go 用 `httptest`），不依赖真实网络或真实凭证。

## 覆盖率报告

三端覆盖率工具都是各语言原生自带，纯本地生成，不需要额外依赖或联网：

- **Node**：`cd node && npm run test:coverage`（Node 内置 test runner 的 `--experimental-test-coverage`），终端直接输出逐文件行/分支/函数覆盖率表格。
- **Java**：`cd java && ./gradlew test jacocoTestReport`（Gradle 内置 JaCoCo 插件），报告在 `java/build/reports/jacoco/test/html/index.html`；已排除 `generated/` 下的 openapi-generator 产物，避免稀释覆盖率数字。
- **Go**：`cd go && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html`；`go/generated` 是纯数据结构，0% 覆盖率是预期结果，不代表未测试。

## 发布与版本管理

三端各自独立 SemVer 版本号。打 tag 触发 CI 自动发布：

| 语言 | 版本号位置 | tag 格式 | 发布方式 |
|---|---|---|---|
| Node | `node/package.json` 的 `version` | `node/vX.Y.Z` | `publish-node.yml`：校验版本号一致 → test/build → `npm publish` |
| Java | `java/build.gradle.kts` 的 `version` | `java/vX.Y.Z` | `publish-java.yml`：校验版本号一致且非 `-SNAPSHOT` → test → `publishToMavenCentral` |
| Go | 无版本文件，tag 即版本 | `go/vX.Y.Z` | 无需发布 workflow，Go module proxy 被动抓取 tag（`go/` 子目录 module，tag 必须带 `go/` 前缀） |

发版流程：改对应版本号文件、合并到 main，再打 tag push 上去，CI 校验一致后自动发布。

发布前置条件（尚未配置）：

- **Node**：仓库 secret `NPM_TOKEN`；包名是 scoped 包 `@qdmp/qdmp-server-sdk`，`@qdmp` 这个 npm org 目前还不存在（已查 `registry.npmjs.org` 确认未注册），发布前需要先创建并让 token 归属的账号拥有该 org 的发布权限。
- **Java**：仓库 secrets `MAVEN_CENTRAL_USERNAME`/`MAVEN_CENTRAL_PASSWORD`/`GPG_SIGNING_KEY`/`GPG_SIGNING_PASSWORD`；且需要在 Central Portal 手动 Add Namespace 验证 `io.github.echotechfe`（组织命名空间不能走 GitHub OAuth 自动验证）。
- **push 到 GitHub**：远程 `origin` 已配置但尚未推送。
