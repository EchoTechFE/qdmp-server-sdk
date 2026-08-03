package io.github.echotechfe.qdmp;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.errors.QdmpTransportException;
import java.io.IOException;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.ResponseBody;
import okio.Buffer;
import okio.BufferedSource;

/**
 * Internal HTTP transport shared by {@link io.github.echotechfe.qdmp.auth.QdmpAuth} and every
 * business group. Not part of the SDK's public API contract in spirit -- it is public only because
 * groups and auth live in separate packages -- callers should go through {@link QdmpClient}
 * instead.
 *
 * <p>Responsible for exactly three things, per the design in {@code shared/openapi.yaml}/{@code
 * shared/generated/route-meta.json}: attaching the right header pair for a given {@link
 * AuthScheme}, encoding query parameters / JSON bodies, and parsing the response. Response parsing
 * tolerates both real envelope shapes -- {@code {code, message, requestId, data}} and the gateway
 * {@code {code, message, details}} -- and treats {@code String(code) == "0"} as the only success
 * signal, never the HTTP status code.
 *
 * <p>The {@link QdmpContext}-taking overloads are a convenience for the business groups: they read
 * the token off the context and send the request exactly once. The SDK never renews a credential on
 * the caller's behalf, so an HTTP 401 reaches the caller untouched.
 */
public final class QdmpTransport {

  private static final MediaType JSON_MEDIA_TYPE = MediaType.get("application/json; charset=utf-8");

  // A malicious or malfunctioning endpoint could otherwise return an arbitrarily large response
  // body, which readBody would buffer entirely into memory before even attempting to parse it.
  private static final long MAX_RESPONSE_BODY_BYTES = 10L * 1024 * 1024;

  // Generated response models have no @JsonIgnoreProperties and openapi.yaml itself flags several
  // `data` schemas as inferred/unverified -- the server is free to add fields we didn't predict.
  // Forward compatibility requires tolerating unknown JSON properties instead of throwing.
  private static final ObjectMapper JSON =
      new ObjectMapper().disable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES);

  private final OkHttpClient httpClient;
  private final HttpUrl baseUrl;
  private final String qdmpVersion;
  private final String appId;

  /**
   * Creates a new transport bound to a fixed base URL and app ID.
   *
   * @param httpClient the underlying OkHttp client
   * @param baseUrl the API host, e.g. {@code https://openapi.qiandao.com/}
   * @param qdmpVersion the value sent as {@code x-echo-qdmp-version} on "standard" scheme calls
   * @param appId the qdmp app ID, sent as {@code x-openapi-app-id} on "genai" scheme calls
   */
  public QdmpTransport(OkHttpClient httpClient, String baseUrl, String qdmpVersion, String appId) {
    HttpUrl parsed = HttpUrl.parse(baseUrl);
    if (parsed == null) {
      throw new IllegalArgumentException("invalid baseUrl: " + baseUrl);
    }
    // QdmpClientConfig.Builder#build() rejects an http:// baseUrl unless the host is
    // loopback/localhost, but that check only runs if the caller goes through QdmpClientConfig.
    // This constructor is public (only because groups/auth live in separate packages from this
    // class) and is reachable directly, so a caller bypassing QdmpClientConfig entirely could
    // otherwise construct a transport pointed at an http:// host and send appSecret/access-token
    // headers in plaintext. Re-run the identical check here, sharing QdmpClientConfig's loopback
    // literal check, for the same defense-in-depth reason the followRedirects(false) call below
    // does not rely on QdmpClient being the only caller.
    if ("http".equalsIgnoreCase(parsed.scheme())
        && !QdmpClientConfig.isLoopbackOrLocalhost(parsed.host())) {
      throw new IllegalArgumentException(
          "baseUrl must use https:// unless the host is loopback/localhost, got: " + baseUrl);
    }
    // Force-disable redirect-following on whatever client we were handed, rather than trusting the
    // caller to have configured it safely: this SDK carries secret credentials as request headers,
    // and this constructor is reachable directly (it is public only because groups/auth live in
    // separate packages), so it must not rely on QdmpClient being the only caller.
    this.httpClient =
        httpClient.newBuilder().followRedirects(false).followSslRedirects(false).build();
    this.baseUrl = parsed;
    this.qdmpVersion = qdmpVersion;
    this.appId = appId;
  }

  /**
   * Issues a GET request and parses the {@code data} field of a successful business envelope into
   * {@code dataType}.
   *
   * @param path the operation path, e.g. {@code /user/v1/me}
   * @param scheme which header pair to attach
   * @param accessToken the token to send, or {@code null} for {@link AuthScheme#NO_AUTH}
   * @param query optional query parameters; values may be scalars or {@link List}s (repeated
   *     params)
   * @param dataType the response {@code data} POJO type
   * @param <T> the response {@code data} type
   * @return the deserialized {@code data} payload
   */
  public <T> T get(
      String path,
      AuthScheme scheme,
      String accessToken,
      Map<String, Object> query,
      Class<T> dataType) {
    HttpUrl.Builder urlBuilder = pathUrlBuilder(path);
    if (query != null) {
      addQueryParams(urlBuilder, query);
    }
    Request.Builder requestBuilder = new Request.Builder().url(urlBuilder.build()).get();
    applyAuthHeaders(requestBuilder, scheme, accessToken);
    return execute(requestBuilder.build(), dataType);
  }

  /**
   * Issues a GET request authenticated with the token carried by a {@link QdmpContext}.
   *
   * @param path the operation path, e.g. {@code /user/v1/me}
   * @param scheme which header pair to attach
   * @param ctx the calling context, carrying the token to authenticate with
   * @param query optional query parameters; values may be scalars or {@link List}s (repeated
   *     params)
   * @param dataType the response {@code data} POJO type
   * @param <T> the response {@code data} type
   * @return the deserialized {@code data} payload
   */
  public <T> T get(
      String path,
      AuthScheme scheme,
      QdmpContext ctx,
      Map<String, Object> query,
      Class<T> dataType) {
    return get(path, scheme, ctx.getAccessToken(), query, dataType);
  }

  /**
   * Issues a POST request with a JSON body and parses the {@code data} field of a successful
   * business envelope into {@code dataType}.
   *
   * @param path the operation path, e.g. {@code /mark/v1/add}
   * @param scheme which header pair to attach
   * @param accessToken the token to send, or {@code null} for {@link AuthScheme#NO_AUTH}
   * @param body the request body, serialized as JSON
   * @param dataType the response {@code data} POJO type
   * @param <T> the response {@code data} type
   * @return the deserialized {@code data} payload
   */
  public <T> T post(
      String path, AuthScheme scheme, String accessToken, Object body, Class<T> dataType) {
    Request.Builder requestBuilder =
        new Request.Builder().url(pathUrlBuilder(path).build()).post(jsonBody(body));
    applyAuthHeaders(requestBuilder, scheme, accessToken);
    return execute(requestBuilder.build(), dataType);
  }

  /**
   * Issues a POST request authenticated with the token carried by a {@link QdmpContext}.
   *
   * @param path the operation path, e.g. {@code /mark/v1/add}
   * @param scheme which header pair to attach
   * @param ctx the calling context, carrying the token to authenticate with
   * @param body the request body, serialized as JSON
   * @param dataType the response {@code data} POJO type
   * @param <T> the response {@code data} type
   * @return the deserialized {@code data} payload
   */
  public <T> T post(
      String path, AuthScheme scheme, QdmpContext ctx, Object body, Class<T> dataType) {
    return post(path, scheme, ctx.getAccessToken(), body, dataType);
  }

  private static RequestBody jsonBody(Object body) {
    try {
      return RequestBody.create(JSON.writeValueAsBytes(body), JSON_MEDIA_TYPE);
    } catch (IOException e) {
      throw new QdmpTransportException("failed to serialize request body", e);
    }
  }

  private HttpUrl.Builder pathUrlBuilder(String path) {
    String relative = path.startsWith("/") ? path.substring(1) : path;
    return baseUrl.newBuilder().addPathSegments(relative);
  }

  private void applyAuthHeaders(Request.Builder builder, AuthScheme scheme, String accessToken) {
    switch (scheme) {
      case STANDARD:
        requireHeaderSafeToken(accessToken);
        builder.header("access-token", accessToken);
        builder.header("x-echo-qdmp-version", qdmpVersion);
        break;
      case GENAI:
        requireHeaderSafeToken(accessToken);
        builder.header("x-openapi-access-token", accessToken);
        builder.header("x-openapi-app-id", appId);
        break;
      case NO_AUTH:
      default:
        break;
    }
  }

  // QdmpTransport is public (only because groups/auth live in separate packages from this class),
  // so it is reachable directly by a caller that bypasses QdmpContext's validation entirely and
  // passes a bare String accessToken straight to get/post. Without this check, such a caller could
  // push a token containing e.g. an embedded newline straight into OkHttp's Request.Builder#header
  // call: OkHttp's own header validation would then reject it, but -- since neither
  // "access-token" nor "x-openapi-access-token" are on OkHttp's hardcoded redaction list
  // (Authorization/Cookie/Proxy-Authorization/Set-Cookie) -- it echoes the entire raw (invalid)
  // header value, including the token, into the thrown IllegalArgumentException's message. Running
  // the same whitelist QdmpContext#of already enforces before ever calling builder.header(...)
  // means that OkHttp code path -- and its token-leaking exception message -- is never reached.
  private static void requireHeaderSafeToken(String accessToken) {
    QdmpContext.requireHeaderSafe(accessToken);
  }

  private void addQueryParams(HttpUrl.Builder urlBuilder, Map<String, Object> query) {
    for (Map.Entry<String, Object> entry : query.entrySet()) {
      Object value = entry.getValue();
      if (value == null) {
        continue;
      }
      if (value instanceof List<?>) {
        for (Object item : (List<?>) value) {
          urlBuilder.addQueryParameter(entry.getKey(), String.valueOf(item));
        }
      } else {
        urlBuilder.addQueryParameter(entry.getKey(), String.valueOf(value));
      }
    }
  }

  private <T> T execute(Request request, Class<T> dataType) {
    try (Response response = httpClient.newCall(request).execute()) {
      if (response.code() >= 300 && response.code() < 400) {
        // Never follow a redirect: this SDK carries secret credentials (access-token /
        // x-openapi-access-token) as request headers, and a redirect could point at an
        // attacker-controlled host that would then receive them. Surface as a transport
        // failure instead -- the message intentionally omits headers/URLs that could carry
        // the access token.
        throw new QdmpTransportException(
            "qdmp request received an HTTP "
                + response.code()
                + " redirect response, which the"
                + " SDK refuses to follow to avoid leaking credentials to an unverified"
                + " destination");
      }
      String bodyString = readBody(response);
      JsonNode root = bodyString.isEmpty() ? JSON.createObjectNode() : JSON.readTree(bodyString);
      JsonNode codeNode = root.get("code");
      String code = codeNode == null ? "" : codeNode.asText();
      if (!"0".equals(code)) {
        String message = root.hasNonNull("message") ? root.get("message").asText() : "";
        String requestId = root.hasNonNull("requestId") ? root.get("requestId").asText() : null;
        throw new QdmpApiError(code, message, requestId, response.code());
      }
      JsonNode dataNode = root.get("data");
      if (dataNode == null || dataNode.isNull()) {
        return null;
      }
      return JSON.treeToValue(dataNode, dataType);
    } catch (IOException e) {
      throw new QdmpTransportException(
          "qdmp request failed: " + request.method() + " " + request.url(), e);
    }
  }

  private String readBody(Response response) throws IOException {
    ResponseBody body = response.body();
    if (body == null) {
      return "";
    }
    BufferedSource source = body.source();
    // Reads at most MAX_RESPONSE_BODY_BYTES + 1 bytes from the underlying connection -- enough to
    // detect an oversized body without ever buffering the rest of it into memory.
    source.request(MAX_RESPONSE_BODY_BYTES + 1);
    Buffer buffer = source.buffer();
    if (buffer.size() > MAX_RESPONSE_BODY_BYTES) {
      throw new QdmpTransportException(
          "qdmp response body exceeded the maximum allowed size of "
              + MAX_RESPONSE_BODY_BYTES
              + " bytes; refusing to buffer it into memory");
    }
    MediaType contentType = body.contentType();
    Charset charset =
        contentType == null ? StandardCharsets.UTF_8 : contentType.charset(StandardCharsets.UTF_8);
    return buffer.readString(charset);
  }
}
