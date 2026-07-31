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
    super(message);
    this.code = code;
    this.requestId = requestId;
    this.httpStatus = httpStatus;
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
