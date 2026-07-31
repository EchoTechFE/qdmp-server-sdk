# 生成代码——请勿手动编辑

`src/main/java/io/github/echotechfe/qdmp/generated/` 目录下大部分文件由 `openapi-generator-cli` 从 `../../shared/openapi.yaml` 生成。重新生成命令：

```bash
rm -rf java/generated
java -jar /tmp/tools/openapi-generator-cli.jar generate \
  -i shared/openapi.yaml -g java -o java/generated \
  --global-property models,modelTests=false,modelDocs=false \
  --model-package=io.github.echotechfe.qdmp.generated \
  --library=resttemplate \
  --additional-properties=hideGenerationTimestamp=true,dateLibrary=java8,openApiNullable=false
```

同一个包下有两个文件**不是** openapi-generator 的产物：`RouteMeta.java` 和 `KnownErrorCodes.java` 由 `../scripts/gen-route-meta.mjs`（运行 `node
scripts/gen-route-meta.mjs`，或 `./gradlew codegen`）从 `shared/generated/route-meta.json` /
`shared/generated/error-codes.json` 生成——这是手写的“第四代生成器”步骤，用来读取 openapi-generator 本身不支持的
`x-qdmp-auth-scheme`/`x-qdmp-token-required`/`x-error-codes` 扩展字段。上面的 `rm -rf java/generated` 会把这两个文件也
一并删掉——重建时务必在 openapi-generator-cli 之后再补跑一次 `node scripts/gen-route-meta.mjs`，不能只跑 openapi-generator-cli。

## 为什么用这些参数（models-only 场景下的坑）

- `--global-property models,modelTests=false,modelDocs=false` ——只生成 models，不生成文档/测试。但光靠这个参数，
  生成器仍然会顺手生成 API 类和 `ApiClient` 相关支持文件，除非再用 `--library` 加以控制；下面的参数就是用来补上
  这个缺口的。
- `--library=resttemplate`（既不是默认的 `okhttp-gson`，也不是 `native`）：
  - `okhttp-gson`（默认值）生成的 model 带 **Gson** 注解（`@SerializedName`、`TypeAdapter` 等）。我们要的是
    Jackson，以匹配 SDK 其他依赖都基于 Jackson 的现状。
  - `native`（Java 11+ `HttpClient`，基于 Jackson）看起来是对的，但它的 model 模板即使加了
    `--global-property models` 也仍然引用 `io.github.echotechfe.qdmp.ApiClient`（用于
    `toUrlQueryString()`/`urlEncode()` 这两个辅助方法）——而这个类属于被跳过的 API/supportingFiles 层，导致
    models-only 产物无法独立编译通过。
  - `resttemplate` 的 model 模板没有这种引用：纯 Jackson 注解的 POJO（`@JsonProperty`、`@JsonInclude` 等），不
    依赖任何非生成类。这是唯一一个能让 models-only 产物独立编译通过的选项。
- `--additional-properties=openApiNullable=false` ——不加这个参数，部分 nullable/`additionalProperties` 字段
  （例如 `GatewayErrorEnvelope.details`）会被包一层 `org.openapitools.jackson.nullable.JsonNullable`，进而引入
  `jackson-databind-nullable` 这个依赖，但对纯 DTO 包来说毫无必要——普通的 nullable 字段就够用了。
- 生成的 model 上带有 `@javax.annotation.Nonnull` / `@Nullable`（JSR-305）以及 `@javax.annotation.Generated`
  （JSR-250）注解——这两个包从 Java 11 起已经从 JDK 里移除，所以引用方模块必须把
  `com.google.code.findbugs:jsr305` 和 `javax.annotation:javax.annotation-api` 放到**编译期**（compile）
  classpath 上（见 `build.gradle.kts`），否则会编译报错 “cannot find symbol”。

## 命名说明

部分 DTO 的类名是按操作名生成的（`UserMe200ResponseAllOfData`、`MarkAdd200ResponseAllOfData` 等），而不是干净的
领域名（`User`、`Mark`）——原因是 `shared/openapi.yaml` 里这些响应的 `data` 对象是按操作内联定义的，而不是通过一个
共享的、有名字的 `components/schemas` 条目定义的。spec 里*确实*命名过的共享 schema（`Island`、`Tag`、`Spu`、
`SpuSummary`、`Rating` 等）就会生成干净的类名。这是 spec 编写方式的选择，不是生成器的怪癖——不要靠手改生成产物
来“修”这个问题；真要修，也应该去改 `shared/openapi.yaml`，把它 `$ref` 到一个命名 schema 后重新生成。
