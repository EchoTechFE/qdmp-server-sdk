package io.github.echotechfe.qdmp;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.errors.QdmpTransportException;
import java.io.IOException;
import java.util.List;
import java.util.Map;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.ResponseBody;

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
 */
public final class QdmpTransport {

  private static final MediaType JSON_MEDIA_TYPE = MediaType.get("application/json; charset=utf-8");

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
    this.httpClient = httpClient;
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
    RequestBody requestBody;
    try {
      requestBody = RequestBody.create(JSON.writeValueAsBytes(body), JSON_MEDIA_TYPE);
    } catch (IOException e) {
      throw new QdmpTransportException("failed to serialize request body", e);
    }
    Request.Builder requestBuilder =
        new Request.Builder().url(pathUrlBuilder(path).build()).post(requestBody);
    applyAuthHeaders(requestBuilder, scheme, accessToken);
    return execute(requestBuilder.build(), dataType);
  }

  private HttpUrl.Builder pathUrlBuilder(String path) {
    String relative = path.startsWith("/") ? path.substring(1) : path;
    return baseUrl.newBuilder().addPathSegments(relative);
  }

  private void applyAuthHeaders(Request.Builder builder, AuthScheme scheme, String accessToken) {
    switch (scheme) {
      case STANDARD:
        builder.header("access-token", accessToken);
        builder.header("x-echo-qdmp-version", qdmpVersion);
        break;
      case GENAI:
        builder.header("x-openapi-access-token", accessToken);
        builder.header("x-openapi-app-id", appId);
        break;
      case NO_AUTH:
      default:
        break;
    }
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
    return body == null ? "" : body.string();
  }
}
