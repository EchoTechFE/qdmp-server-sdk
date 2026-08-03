package io.github.echotechfe.qdmp.errors;

/**
 * Thrown for SDK-local parameter validation failures that never reach the network -- e.g. a
 * missing/empty {@code code} passed to {@code auth.getUserAccessToken} (required for grantType
 * {@code AUTHORIZATION_CODE}) or a missing/empty {@code refreshToken} passed to {@code
 * auth.refreshToken}. Mirrors the Node SDK's {@code QdmpValidationError} (see {@code
 * node/src/errors.ts}) for cross-language behavioral parity. Kept distinct from {@link
 * QdmpApiError}, which always represents an actual server response, and from {@link
 * QdmpTransportException}, which represents a below-HTTP-layer failure.
 */
public final class QdmpValidationError extends RuntimeException {

  private static final long serialVersionUID = 1L;

  /**
   * Creates a new validation error.
   *
   * @param message a description of which argument was missing/invalid and why
   */
  public QdmpValidationError(String message) {
    super(message);
  }
}
