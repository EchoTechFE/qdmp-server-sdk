package io.github.echotechfe.qdmp.errors;

/**
 * Thrown when a qdmp OpenAPI call fails below the HTTP layer (connection refused, timeout, I/O
 * error, malformed response body). Distinct from {@link QdmpApiError}, which represents a response
 * the server actually sent back with a non-success business code.
 */
public final class QdmpTransportException extends RuntimeException {

  private static final long serialVersionUID = 1L;

  /**
   * Creates a new transport exception.
   *
   * @param message a description of the failure
   * @param cause the underlying cause, typically an {@link java.io.IOException}
   */
  public QdmpTransportException(String message, Throwable cause) {
    super(message, cause);
  }

  /**
   * Creates a new transport exception with no underlying {@link Throwable} cause -- used for
   * failures detected by inspecting an otherwise-successful response body (e.g. a business envelope
   * reporting {@code code:"0"} but a structurally malformed {@code data} payload), as opposed to an
   * I/O-level failure.
   *
   * @param message a description of the failure
   */
  public QdmpTransportException(String message) {
    super(message);
  }
}
