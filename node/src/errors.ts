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

/** Upper bound on how much of a server-supplied string is echoed into an
 * Error's message — a server (or a compromised intermediary) must never be
 * able to force an unbounded string into our caller's memory/logs via
 * Error.message. */
const MAX_SANITIZED_MESSAGE_LENGTH = 500;

/** Matches the first C0 control character (0x00-0x1F) or DEL (0x7F) in a
 * string — most notably CR and LF. */
const CONTROL_CHAR_PATTERN = new RegExp(
  '[' +
    String.fromCharCode(...Array.from({length: 32}, (_, i) => i)) +
    String.fromCharCode(127) +
    ']',
);

/** Sanitizes a server-supplied string before it is ever interpolated into an
 * Error's message. A malicious/compromised server could embed CR/LF into
 * `message` to forge fake extra log lines once a caller writes
 * error.message to a single-line log (log injection). Replacing the control
 * character in place would still leave every byte the attacker chose intact
 * and visible right next to it, so instead this truncates the string at the
 * first control character — nothing the attacker placed after an injected
 * CR/LF is ever included, forged log line or not. Also enforces a hard
 * length cap so an arbitrarily long message can't bloat/DoS a caller's
 * memory or logs. */
function sanitizeForErrorMessage(value: string): string {
  const controlCharIndex = value.search(CONTROL_CHAR_PATTERN);
  const withoutControlChars =
    controlCharIndex === -1 ? value : value.slice(0, controlCharIndex);
  return withoutControlChars.length > MAX_SANITIZED_MESSAGE_LENGTH
    ? withoutControlChars.slice(0, MAX_SANITIZED_MESSAGE_LENGTH) + '…'
    : withoutControlChars;
}

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
    // `code` is an intentionally open `string | number` union (see file
    // header) — when it's a string, it is just as server-controlled as
    // `message` and must go through the same sanitization before being
    // interpolated into this Error's message. A numeric `code` can't carry
    // CR/LF or excessive length, so it's used as-is.
    const sanitizedCode =
      typeof init.code === 'string'
        ? sanitizeForErrorMessage(init.code)
        : init.code;
    super(
      `qdmp API error (code=${sanitizedCode}, httpStatus=${init.httpStatus}): ${sanitizeForErrorMessage(init.message)}`,
    );
    this.name = 'QdmpApiError';
    // Store the same sanitized value used above for the message, not the
    // raw init.code — this.code and toJSON().code are public surfaces a
    // caller may log/serialize directly, bypassing .message entirely.
    this.code = sanitizedCode;
    this.httpStatus = init.httpStatus;
    // requestId is just as server-controlled as message/code — sanitize it
    // too, since callers may log it directly (or via toJSON()) without ever
    // going through Error.message.
    this.requestId =
      init.requestId !== undefined
        ? sanitizeForErrorMessage(init.requestId)
        : undefined;
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

export interface QdmpTransportErrorOptions {
  cause?: unknown;
  httpStatus?: number;
}

/** Thrown for transport-level failures that must never be treated as a
 * business response — e.g. an unexpected 3xx redirect, or a malformed
 * "success" envelope that fails local shape validation before it is ever
 * cached or returned to a caller. Never include header values (accessToken,
 * appSecret, refreshToken, Location) in the message. */
export class QdmpTransportError extends Error {
  readonly cause?: unknown;
  readonly httpStatus?: number;

  constructor(message: string, options?: QdmpTransportErrorOptions) {
    super(message);
    this.name = 'QdmpTransportError';
    this.cause = options?.cause;
    this.httpStatus = options?.httpStatus;
    Object.setPrototypeOf(this, QdmpTransportError.prototype);
  }

  override toString(): string {
    return this.message;
  }
}
