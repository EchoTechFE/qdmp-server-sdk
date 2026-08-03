import {inspect} from 'node:util';

/**
 * Attaches a non-enumerable [util.inspect.custom] renderer to `value` so
 * that `console.log(value)`/`util.inspect(value)` — an easy accidental
 * debug-print path — never shows the raw accessToken/refreshToken. This is
 * deliberately done via Object.defineProperty on an already-constructed,
 * already-typed value rather than as an object-literal property: adding
 * `[inspect.custom]: ...` directly to a `UserAccessTokenData`/
 * `RefreshTokenData` literal would trip TypeScript's excess-property check
 * (none of those interfaces declares a symbol-keyed member).
 *
 * This must never affect JSON.stringify(value) or direct property access
 * (value.accessToken): JSON.stringify only consults toJSON(), not
 * util.inspect.custom, and the defined property here is non-enumerable so
 * it does not add a spurious key to the object either. Persisting the real
 * values is the entire point of getUserAccessToken()/refreshToken()
 * existing, so that path must stay untouched.
 */
export function withRedactedInspect<T extends object>(
  value: T,
  render: () => string,
): T {
  Object.defineProperty(value, inspect.custom, {
    value: render,
    enumerable: false,
  });
  return value;
}
