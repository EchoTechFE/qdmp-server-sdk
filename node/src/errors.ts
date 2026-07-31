/**
 * Error types for qdmp-server-sdk.
 *
 * Design constraints:
 * - Success/failure is determined solely by `String(code) === '0'` in the
 *   response envelope, never by HTTP status: refreshToken expiry is a real
 *   HTTP 200 + code=10008, while an invalid/revoked access-token is a real
 *   HTTP 401 + code=10005.
 * - `code` is an intentionally open `string | number` union, not a closed
 *   enum — undocumented codes are known to occur (10005, 20000, and small
 *   gateway-layer integers like 2 / 13).
 * - Nothing here ever echoes back accessToken / appSecret / refreshToken.
 */

export type QdmpErrorCode = string | number;

export interface QdmpApiErrorInit {
  code: QdmpErrorCode;
  message: string;
  httpStatus: number;
  requestId?: string;
}

/** Thrown for any qdmp OpenAPI call that did not succeed, whether the
 * failure surfaced as a business envelope `{code,message,requestId,data}`
 * or a gateway envelope `{code,message,details}`. */
export class QdmpApiError extends Error {
  readonly code: QdmpErrorCode;
  readonly httpStatus: number;
  readonly requestId?: string;

  constructor(init: QdmpApiErrorInit) {
    super(
      `qdmp API error (code=${init.code}, httpStatus=${init.httpStatus}): ${init.message}`,
    );
    this.name = 'QdmpApiError';
    this.code = init.code;
    this.httpStatus = init.httpStatus;
    this.requestId = init.requestId;
    // Restore the prototype chain — needed because targets below ES2015
    // (and some transpilers) don't do this automatically when extending
    // built-ins like Error.
    Object.setPrototypeOf(this, QdmpApiError.prototype);
  }

  override toString(): string {
    return this.message;
  }

  toJSON(): Record<string, unknown> {
    return {
      name: this.name,
      code: this.code,
      message: this.message,
      httpStatus: this.httpStatus,
      requestId: this.requestId,
    };
  }
}

/** Thrown for SDK-local parameter validation failures (e.g. a missing
 * required `ctx.accessToken`) that never reach the network. Kept distinct
 * from QdmpApiError, which always represents an actual server response. */
export class QdmpValidationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'QdmpValidationError';
    Object.setPrototypeOf(this, QdmpValidationError.prototype);
  }
}
