/**
 * Core HTTP transport + response-envelope parsing, shared by auth.ts and
 * every groups/*.ts business method.
 *
 * Uses Node's built-in global `fetch` (Node ≥18, stable since ≥21) instead
 * of importing `fetch` from the `undici` package — one fewer runtime
 * dependency. Node's global fetch does honor an explicitly-passed
 * `dispatcher` option (verified against this project's exact Node/undici
 * versions); `undici` stays a devDependency purely for its `Dispatcher`
 * type and the test suite's `MockAgent`.
 */
import type {Dispatcher} from 'undici';

import {QdmpApiError, QdmpTransportError} from './errors.js';
import type {QdmpAuthScheme, QdmpHttpMethod} from './generated/route-meta.js';

export interface HttpClientConfig {
  baseUrl: string;
  dispatcher?: Dispatcher;
}

/** Upper bound on how many response body bytes we'll read before giving up.
 * The qdmp OpenAPI response is a business JSON envelope, not a bulk data
 * transfer — an oversized body (whether from a misbehaving server or a
 * malicious intermediary) should never be fully buffered into memory just to
 * find out it doesn't fit. Content-Length can't be trusted for this (it may
 * be absent, or simply wrong), so the actual bytes read off the stream are
 * counted as they arrive. */
const MAX_RESPONSE_BODY_BYTES = 10 * 1024 * 1024;

/** Reads `response.body` chunk-by-chunk, counting bytes as they arrive so a
 * body exceeding MAX_RESPONSE_BODY_BYTES is rejected — and the underlying
 * stream cancelled — well before it would ever be handed to JSON.parse(). */
async function readBoundedJsonBody(
  response: Response,
  httpStatus: number,
): Promise<unknown> {
  const body = response.body;
  if (!body) {
    // No body stream (e.g. some 204-like responses); treat as empty.
    return JSON.parse('');
  }
  const reader = body.getReader();
  const chunks: Uint8Array[] = [];
  let totalBytes = 0;
  for (;;) {
    const {done, value} = await reader.read();
    if (done) {
      break;
    }
    totalBytes += value.byteLength;
    if (totalBytes > MAX_RESPONSE_BODY_BYTES) {
      await reader.cancel().catch(() => {
        // best-effort connection cleanup; must not mask the size error
      });
      throw new QdmpTransportError(
        `response body exceeded the maximum allowed size of ${MAX_RESPONSE_BODY_BYTES} bytes`,
        {httpStatus},
      );
    }
    chunks.push(value);
  }
  const combined = new Uint8Array(totalBytes);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.byteLength;
  }
  const text = new TextDecoder('utf-8').decode(combined);
  return JSON.parse(text);
}

export type QueryValue = string | number | boolean | string[] | undefined;

export interface HttpRequestOptions {
  method: QdmpHttpMethod;
  path: string;
  authScheme: QdmpAuthScheme;
  /** Required unless authScheme is 'noAuth'. */
  accessToken?: string;
  /** Required only for the 'genai' authScheme (x-openapi-app-id header). */
  appId?: string;
  /** Required only for the 'standard' authScheme (x-echo-qdmp-version header). */
  qdmpVersion?: string;
  query?: Record<string, QueryValue>;
  body?: unknown;
}

function appendQuery(url: URL, query?: Record<string, QueryValue>): void {
  if (!query) {
    return;
  }
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) {
      continue;
    }
    if (Array.isArray(value)) {
      for (const item of value) {
        url.searchParams.append(key, item);
      }
    } else {
      url.searchParams.append(key, String(value));
    }
  }
}

function buildHeaders(options: HttpRequestOptions): Record<string, string> {
  const headers: Record<string, string> = {};
  if (options.body !== undefined) {
    headers['content-type'] = 'application/json';
  }
  if (options.authScheme === 'standard') {
    headers['access-token'] = options.accessToken ?? '';
    headers['x-echo-qdmp-version'] = options.qdmpVersion ?? '1.0';
  } else if (options.authScheme === 'genai') {
    headers['x-openapi-access-token'] = options.accessToken ?? '';
    headers['x-openapi-app-id'] = options.appId ?? '';
  }
  return headers;
}

/**
 * Parses one of the two known response envelope shapes:
 *   - business envelope: {code, message, requestId, data}
 *   - gateway envelope:   {code, message, details}
 * and resolves with `data` on success, or throws QdmpApiError on failure.
 *
 * Success is decided ONLY by `String(code) === '0'` — never by httpStatus
 * (refreshToken expiry is a real HTTP 200 + code=10008; an invalid
 * access-token is a real HTTP 401 + code=10005).
 */
function parseEnvelope<T>(payload: unknown, httpStatus: number): T {
  if (typeof payload !== 'object' || payload === null) {
    throw new QdmpApiError({
      code: 'QDMP_SDK_INVALID_RESPONSE_SHAPE',
      message: 'response body is not a JSON object',
      httpStatus,
    });
  }
  const body = payload as Record<string, unknown>;
  const rawCode: unknown = body.code;
  if (rawCode === undefined) {
    throw new QdmpApiError({
      code: 'QDMP_SDK_INVALID_RESPONSE_SHAPE',
      message: 'response body is missing the "code" field',
      httpStatus,
    });
  }
  // `body.code` was previously narrowed with `as string | number` — a type
  // assertion, not a runtime check. A server (or compromised intermediary)
  // returning e.g. an array/object for "code" would sail through that
  // assertion, bypass QdmpApiError's string-only sanitization (which only
  // triggers for typeof === 'string'), and have its unbounded
  // Array.prototype.toString()/Object.prototype.toString() output
  // interpolated straight into the thrown error's message/.code/.toJSON().
  if (typeof rawCode !== 'string' && typeof rawCode !== 'number') {
    throw new QdmpApiError({
      code: 'QDMP_SDK_INVALID_RESPONSE_SHAPE',
      message: 'response body\'s "code" field is neither a string nor a number',
      httpStatus,
    });
  }
  const code = rawCode;
  const message = typeof body.message === 'string' ? body.message : '';
  if (String(code) !== '0') {
    const requestId =
      typeof body.requestId === 'string' ? body.requestId : undefined;
    throw new QdmpApiError({code, message, requestId, httpStatus});
  }
  return (body.data ?? {}) as T;
}

export class HttpClient {
  private readonly baseUrl: string;
  private readonly dispatcher: Dispatcher | undefined;

  constructor(config: HttpClientConfig) {
    this.baseUrl = config.baseUrl;
    this.dispatcher = config.dispatcher;
  }

  /** Sends the request exactly once and returns the parsed `data` — or
   * throws. The SDK never retries: an expired/rejected access-token surfaces
   * as a QdmpApiError (HTTP 401 + business code 10005/10006) for the caller
   * to handle, e.g. by calling `auth.refreshToken()` themselves. */
  async request<T>(options: HttpRequestOptions): Promise<T> {
    const url = new URL(options.path, this.baseUrl);
    appendQuery(url, options.query);

    const response = await fetch(url, {
      method: options.method,
      headers: buildHeaders(options),
      body:
        options.body !== undefined ? JSON.stringify(options.body) : undefined,
      dispatcher: this.dispatcher,
      // Never auto-follow redirects: a 3xx response is returned to us as-is
      // instead of fetch silently re-issuing the request (potentially with
      // our access-token / app-secret headers) against a different origin.
      redirect: 'manual',
    });

    const httpStatus = response.status;
    if (httpStatus >= 300 && httpStatus < 400) {
      // Never surface anything derived from the response (e.g. the
      // Location header) in this error: the redirect target is
      // attacker-influenced and could reflect request data — including a
      // token — back into a header value that would otherwise end up in
      // this error's message/logs.
      try {
        await response.body?.cancel();
      } catch {
        // best-effort connection cleanup; must not mask the redirect error
      }
      throw new QdmpTransportError(
        `received an unexpected HTTP ${httpStatus} redirect response`,
        {httpStatus},
      );
    }
    let payload: unknown;
    try {
      payload = await readBoundedJsonBody(response, httpStatus);
    } catch (err) {
      if (err instanceof QdmpTransportError) {
        throw err;
      }
      throw new QdmpApiError({
        code: 'QDMP_SDK_INVALID_JSON_RESPONSE',
        message: 'response body could not be parsed as JSON',
        httpStatus,
      });
    }
    return parseEnvelope<T>(payload, httpStatus);
  }
}
