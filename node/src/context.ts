import {QdmpValidationError} from './errors.js';
import type {QdmpContext} from './types.js';

/** Validates that a caller passed a non-empty accessToken and returns it.
 * Throws QdmpValidationError (a purely local, no-network error) otherwise —
 * every business group method must call this before touching HttpClient, so
 * a missing token never results in an outbound request. */
export function requireAccessToken(
  ctx: QdmpContext | undefined | null,
): string {
  const accessToken = ctx?.accessToken;
  if (!accessToken) {
    throw new QdmpValidationError(
      'ctx.accessToken is required for this qdmp-server-sdk call',
    );
  }
  return accessToken;
}
