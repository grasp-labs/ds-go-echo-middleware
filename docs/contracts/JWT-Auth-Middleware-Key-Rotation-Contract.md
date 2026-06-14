# JWT Auth Middleware — Key-Rotation Contract (reference for AI Commons API services)

Every AI Commons EKS API service (`ai-gateway`, `state`, `file`, `config`, …)
validates the identity-server's RS256 access tokens in its auth middleware. This
document is the **contract that middleware must satisfy so a signing-key rotation
on the identity-server never causes a verification outage** — no redeploys, no
config pushes, no coordination.

It is the consumer-side counterpart to the producer-side runbook
[`JWT-Key-Rotation.md`](../training/JWT-Key-Rotation.md), and the rotation-specific
deep-dive of section 4 of
[`RFC9728-Protected-Resource-Metadata.md`](./RFC9728-Protected-Resource-Metadata.md).
If you only read one rule, read this: **resolve the verifying key by the token's
`kid` from the live JWKS — never pin a single key.**

**Base URLs:**

| Env   | Issuer (`iss`)                    | JWKS (`jwks_uri`)                                            |
| ----- | --------------------------------- | ----------------------------------------------------------- |
| prod  | `https://auth.grasp-daas.com`     | `https://auth.grasp-daas.com/oauth/.well-known/jwks.json`     |
| dev   | `https://auth-dev.grasp-daas.com` | `https://auth-dev.grasp-daas.com/oauth/.well-known/jwks.json` |
| local | `http://localhost:8000`           | `http://localhost:8000/oauth/.well-known/jwks.json`           |

---

## 1. What the identity-server guarantees (producer side)

When validating tokens you may rely on these invariants. They are what make the
contract in section 2 sufficient:

1. **Every access token carries a `kid`** in its JWT header identifying the
   signing key.
2. **The JWKS at `jwks_uri` always contains the key for every currently-valid
   token.** During a rotation it publishes **both** the new (active) key and the
   retiring key(s), each with a distinct `kid`.
3. **The new key is published in the JWKS before it is used to sign.** A token
   you receive will never reference a `kid` that the JWKS has not yet advertised
   (assuming you re-read the JWKS — see §3).
4. **Retiring keys stay published for the overlap window** — at least the
   longest-lived token's lifetime (currently the refresh-token lifetime, ~1 day;
   operators round up, e.g. 48h). After that the old `kid` disappears from the
   JWKS and its tokens are expected to be expired/rejected.
5. **Algorithm is RS256.** The `kid` is a stable thumbprint of the public key.
6. **Emergency rotation may skip the overlap.** On key compromise the operator
   pulls the old key immediately; tokens signed by it then fail — by design.

> The headline consequence: the rotation **overlap window (≥ ~1 day) is vastly
> larger than any sane JWKS cache TTL (minutes)**. A middleware that refreshes
> its JWKS on a short TTL — or on an unknown `kid` — will always have learned the
> new key long before it must verify a token signed by it.

---

## 2. What the middleware MUST do (the contract)

- **MUST** read the `kid` from the token header and select the matching key from
  the JWKS. **MUST NOT** hardcode, env-bake, or otherwise pin a single public key
  out-of-band.
- **MUST** tolerate **multiple keys in the JWKS at once** and pick by `kid`
  (the set also contains the OIDC id_token key — ignore keys whose `kid` you
  don't need; never assume the set has exactly one key).
- **MUST** fetch the JWKS over HTTPS from the issuer's `jwks_uri` and **cache**
  it (see §3). **MUST NOT** fetch the JWKS on every request.
- **MUST** refresh the JWKS when it encounters a **`kid` it does not have
  cached**, then retry key resolution once (subject to the cooldown in §3).
- **MUST** verify, after signature: `iss` == your env's issuer, `aud` contains
  your exact `resource` id (audience-confusion guard), and `exp`/`nbf` with small
  clock-skew leeway (≤ 60s).
- **MUST** reject (`401`) — not 5xx — when a token references a `kid` that is
  absent from a freshly-refreshed JWKS. An unknown `kid` is an invalid token, not
  a server error.
- **SHOULD** accept `RS256` only. **MUST NOT** accept `alg: none` or HS* (an
  attacker could otherwise sign with the public key as an HMAC secret).
- **SHOULD NOT** require a redeploy/restart to pick up a rotation. Rotation
  handling is a runtime cache concern, not a build-time concern.

If all of the above hold, a routine rotation is invisible to your service.

---

## 3. JWKS caching & refresh strategy

The middleware needs a small key cache keyed by `kid`. Three knobs:

| Knob | Recommendation | Why |
| --- | --- | --- |
| **Cache TTL** (max age) | 5–10 min | Bounds how long a *removed* key lingers and how soon a *new* key is seen proactively. Far shorter than the rotation overlap, so correctness never depends on it. |
| **Refresh on unknown `kid`** | Yes (force a fetch on cache miss) | Picks up a brand-new key immediately, before the TTL elapses — the belt to the TTL's suspenders. |
| **Refresh cooldown** (min interval between fetches) | 30–60 s | Prevents a flood of bogus `kid`s (or an outage of the IdP) from hammering `jwks_uri`. Bounded retries; fail closed with `401` if still unresolved. |

Additional guidance:

- **Negative outcome = `401`, not retry storm.** After one cooldown-gated
  refresh, if the `kid` is still unknown, reject the token.
- **Serve stale on fetch failure.** If `jwks_uri` is briefly unreachable, keep
  using the last good key set rather than failing all requests; only the *new*
  key is unavailable, and the overlap window gives you time.
- **Add jitter** to TTL-driven refreshes across instances so a fleet doesn't
  stampede the IdP at the same instant.
- **Don't cache forever and don't cache per-process-lifetime only.** Both extremes
  break rotation: forever never sees removals; per-request never caches.

> Most mature libraries implement this pattern for you. Prefer them over a
> hand-rolled cache (see §4). If you must hand-roll, copy the pseudocode in §4c.

---

## 4. Reference implementations (rotation-focused)

### 4a. Python — `PyJWT` `PyJWKClient`

`PyJWKClient` caches the JWK set and resolves by `kid`. Configure a bounded
lifespan so rotations are seen promptly; it refetches when the cache is stale.

```python
import jwt
from jwt import PyJWKClient, PyJWKClientError

ISSUER = "https://auth.grasp-daas.com"            # per env
RESOURCE = "https://grasp-daas.com/api/state/v1"  # per env
JWKS_URI = f"{ISSUER}/oauth/.well-known/jwks.json"

# lifespan: cache the JWK set for 5 min, then refetch (picks up new/removed kids).
_jwks = PyJWKClient(JWKS_URI, cache_keys=True, lifespan=300)


def verify(token: str) -> dict:
    try:
        signing_key = _jwks.get_signing_key_from_jwt(token)  # selects by kid
        return jwt.decode(
            token,
            signing_key.key,
            algorithms=["RS256"],          # RS256 only — never `none`/HS*
            issuer=ISSUER,                 # enforce iss
            audience=RESOURCE,             # enforce aud == your resource
            leeway=30,                     # clock-skew tolerance
            options={"require": ["exp", "iss", "aud"]},
        )
    except PyJWKClientError as exc:
        # kid not found even after a refresh → invalid token, not a 5xx.
        raise Unauthorized("invalid_token", str(exc))
    except jwt.PyJWTError as exc:
        raise Unauthorized("invalid_token", str(exc))
```

> If your `PyJWT` version does not force a refetch on an unknown `kid`, the 5 min
> `lifespan` still guarantees the new key is picked up within the TTL — which is
> minutes vs. the ~1-day overlap. For zero-lag pickup, wrap `get_signing_key_*`
> so that a `PyJWKClientError` triggers one `_jwks.fetch_data()` + retry, gated
> by a 30–60 s cooldown.

### 4b. Node — `jose` `createRemoteJWKSet`

`jose` resolves by `kid`, caches for `cacheMaxAge`, and **automatically refetches
when it sees an unknown `kid`**, rate-limited by `cooldownDuration`. This matches
the contract out of the box.

```js
import { createRemoteJWKSet, jwtVerify } from "jose";

const ISSUER = "https://auth.grasp-daas.com";             // per env
const RESOURCE = "https://grasp-daas.com/api/state/v1";   // per env

const JWKS = createRemoteJWKSet(
  new URL(`${ISSUER}/oauth/.well-known/jwks.json`),
  {
    cacheMaxAge: 600_000,    // 10 min cache
    cooldownDuration: 30_000 // min 30s between refetches on unknown kid
  }
);

export async function verify(token) {
  try {
    const { payload } = await jwtVerify(token, JWKS, {
      issuer: ISSUER,        // enforce iss
      audience: RESOURCE,    // enforce aud == your resource
      algorithms: ["RS256"], // RS256 only
      clockTolerance: 30,    // seconds
    });
    return payload;
  } catch (e) {
    throw new Unauthorized("invalid_token", e.message); // 401, not 5xx
  }
}
```

### 4c. Framework-agnostic pseudocode (hand-rolled cache)

```text
state:
    keys_by_kid = {}          # kid -> public key
    fetched_at  = 0
    TTL         = 300s
    COOLDOWN    = 30s
    last_fetch  = 0

fn get_key(kid):
    if kid in keys_by_kid and (now - fetched_at) < TTL:
        return keys_by_kid[kid]
    # cache miss or stale: refresh, but not more often than COOLDOWN
    if (now - last_fetch) >= COOLDOWN:
        try:
            keys_by_kid = fetch_jwks(JWKS_URI)   # {kid: key} for ALL keys
            fetched_at  = now
        finally:
            last_fetch  = now                    # cooldown applies even on failure
    return keys_by_kid.get(kid)                  # may be None

fn verify(token):
    kid = header(token).kid
    key = get_key(kid)
    if key is None:           return 401 invalid_token   # unknown kid
    claims = rs256_verify(token, key)                    # signature
    assert claims.iss == ISSUER
    assert RESOURCE in as_list(claims.aud)               # audience-confusion guard
    assert not expired(claims, leeway=30s)
    return claims
```

---

## 5. Anti-patterns (these break on rotation)

- **Pinning the public key** in an env var / secret / config map and verifying
  against it. The first rotation breaks every token.
- **Caching the JWKS forever** (no TTL, no unknown-`kid` refresh). New keys are
  never learned; rotation breaks new tokens.
- **Fetching the JWKS on every request.** Correct but a self-inflicted DoS on the
  IdP and a latency tax; also fragile if `jwks_uri` blips.
- **Assuming a single key** in the JWKS (e.g. `keys[0]`). The set legitimately
  holds the OIDC key plus active + retiring access-token keys.
- **Returning `5xx` for an unknown `kid`.** It is an invalid token → `401`.
- **Accepting `alg` from the token** (`none`, HS*). Always constrain to `RS256`.
- **Skipping `aud`/`iss` checks** because the signature passed. A valid signature
  for another service's token is still the wrong audience.

---

## 6. Checklist per service

- [ ] Resolve the verifying key by `kid` from the live JWKS; no pinned key.
- [ ] Cache the JWKS with a 5–10 min TTL **and** refresh on unknown `kid`
      (cooldown 30–60 s).
- [ ] Constrain to `RS256`; reject `none`/HS*.
- [ ] Enforce `iss` == your env issuer and `aud` contains your exact `resource`.
- [ ] Apply ≤ 60 s clock-skew leeway on `exp`/`nbf`.
- [ ] Unknown `kid` / bad signature / wrong `aud` → `401` (with
      `WWW-Authenticate`, per the RFC 9728 doc), never `5xx`.
- [ ] Serve last-good keys if `jwks_uri` is briefly unreachable.
- [ ] Verify rotation works end-to-end against `dev` before relying on it in
      `prod` (rotate dev per [`JWT-Key-Rotation.md`](../training/JWT-Key-Rotation.md) and
      confirm no 401 blip).
```
