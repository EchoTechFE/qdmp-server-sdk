package io.github.echotechfe.qdmp.errors;

/**
 * Thrown when a qdmp OpenAPI call reports a business-level failure. The SDK determines success
 * purely by inspecting the response body's {@code code} field ({@code String(code) == "0"}) and
 * never by trusting the HTTP status code alone -- a {@code refreshToken} expiry is reported as HTTP
 * 200 with {@code code:10008}, while a revoked access token is a real HTTP 401 with {@code
 * code:10005}. {@code code} is intentionally an open string, not a closed enum: the qdmp API is
 * known to return codes outside the documented set (e.g. {@code 13}/{@code 2} from the gateway
 * layer, or {@code 20000} for "user not found").
 *
 * <p>{@link #toString()} and {@link #getMessage()} never echo back the request's access token or
 * app secret -- only the {@code code}/{@code message}/{@code requestId}/{@code httpStatus} fields
 * taken from the server's response are included.
 */
public final class QdmpApiError extends RuntimeException {

  private static final long serialVersionUID = 1L;

  // Bounds how much of a server-controlled string (message/requestId) this SDK will hold onto and
  // echo back via toString()/getMessage() -- a malicious or buggy upstream could otherwise return
  // hundreds of KB in a field that ends up written to logs in full.
  private static final int MAX_SANITIZED_FIELD_LENGTH = 2000;

  private final String code;
  private final String requestId;
  private final int httpStatus;

  /**
   * Creates a new API error.
   *
   * @param code the business status code reported by the server, stringified
   * @param message the human-readable message reported by the server
   * @param requestId the request trace ID, present only on the business envelope shape; may be
   *     {@code null}
   * @param httpStatus the actual HTTP status code of the response (not necessarily an error status
   *     -- see class Javadoc)
   */
  public QdmpApiError(String code, String message, String requestId, int httpStatus) {
    super(sanitize(message));
    this.code = sanitize(code);
    this.requestId = sanitize(requestId);
    this.httpStatus = httpStatus;
  }

  // code/message/requestId are taken verbatim from the server's response body -- a malicious or
  // man-in-the-middle response containing e.g. raw \r\n could otherwise inject fake log lines into
  // anything that logs this exception via toString()/getMessage(). code is an intentionally open
  // string (see class Javadoc), not a closed enum, so it is just as attacker-influenced as
  // message/requestId and must go through the exact same sanitize() gate. Stripping ASCII control
  // characters (< 0x20) and DEL (0x7f), and bounding the length, applies to all three fields before
  // they are ever stored, so every accessor (getCode(), getMessage(), the requestId field,
  // toString()) sees the sanitized value.
  private static String sanitize(String value) {
    if (value == null) {
      return null;
    }
    String truncated =
        value.length() > MAX_SANITIZED_FIELD_LENGTH
            ? value.substring(0, MAX_SANITIZED_FIELD_LENGTH)
            : value;
    StringBuilder sanitized = new StringBuilder(truncated.length());
    for (int i = 0; i < truncated.length(); i++) {
      char c = truncated.charAt(i);
      sanitized.append(c < 0x20 || c == 0x7f ? ' ' : c);
    }
    return sanitized.toString();
  }

  /**
   * Returns the business status code reported by the server.
   *
   * @return the stringified business code, e.g. {@code "10005"}
   */
  public String getCode() {
    return code;
  }

  /**
   * Returns the request trace ID, if the server's response included one.
   *
   * @return the request ID, or {@code null} if the response used the gateway error envelope
   */
  public String getRequestId() {
    return requestId;
  }

  /**
   * Returns the actual HTTP status code of the response.
   *
   * @return the HTTP status code
   */
  public int getHttpStatus() {
    return httpStatus;
  }

  @Override
  public String toString() {
    return "QdmpApiError{code="
        + code
        + ", message="
        + getMessage()
        + ", requestId="
        + requestId
        + ", httpStatus="
        + httpStatus
        + "}";
  }
}
