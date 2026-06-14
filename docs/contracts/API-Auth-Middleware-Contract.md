# API Auth Middleware Contract — user- and app-based JWT, opt-in audience check

> **Audience:** engineers building the auth middleware for any resource API that
> consumes identity-server access tokens (`ai-gateway`, `state`, `file`,
> `config`, MCP servers, …).
>
> This is the **identity/authorization** contract: how to validate a token, how
> to tell a **human** apart from an **app**, what to put on your request
> context, and how to **opt in** to per-resource audience enforcement.
>
> It is the companion to the rotation-specific
> [`JWT-Auth-Middleware-Key-Rotation-Contract.md`](./JWT-Auth-Middleware-Key-Rotation-Contract.md)
> (how to survive a signing-key rotation). **Both apply.** If they ever seem to
> conflict, the rotation contract wins on key handling and this one wins on
> claims handling.

---

## 1. What every token guarantees

The identity server signs **RS256** access tokens with a `kid` header. After you
verify the signature (per the rotation contract), you can rely on this claim
set:

| Claim | Type | Meaning |
| ----- | ---- | ------- |
| `iss` | string | `https://auth.grasp-daas.com` (per env) — **must** match your env issuer |
| `sub` | string | **identity**: user **email** if `cls=user`, app **client_id** if `cls=app` |
| `cls` | string | `"user"` or `"app"` — principal *kind* (see §3) |
| `rsc` | string | `"<tenant_id>:<tenant_name>"` — **tenant binding** (parse the UUID before the first `:`) |
| `aud` | string or string[] | intended audience(s) — see §4 |
| `rol` | string[] | coarse flags: subset of `root`/`staff`/`superuser` (**not** permissions) |
| `exp`/`nbf`/`iat` | int | standard time bounds |
| `jti` | string | unique token id |
| `ver` | string | claim-schema version (`2.0.0`) |

> **The IdP asserts identity, not permission.** It tells you *who* (`sub`),
> *which tenant* (`rsc`), and *what kind* (`cls`). Your service resolves
> **permissions** by asking the **entitlement server** "what can `sub` in tenant
> `<tenant_id>` do?" — never by trusting a permission list in the token (there
> isn't one beyond `rol`).

---

## 2. The contract (MUST / SHOULD)

A conforming middleware:

- **MUST** verify the RS256 signature by resolving the key via the token's `kid`
  from the live JWKS (see the rotation contract). **MUST NOT** pin a key or
  accept `alg: none`/HS\*.
- **MUST** enforce `iss` == your environment's issuer.
- **MUST** enforce `exp`/`nbf` with ≤ 60 s clock-skew leeway.
- **MUST** parse `cls` and expose a normalized principal (§3). A token with an
  unrecognized `cls` is invalid → `401`.
- **MUST** parse the **tenant id** from `rsc` (substring before the first `:`)
  and scope every downstream authorization/data access to it. Cross-tenant
  access **MUST** be impossible from a token alone.
- **MUST** enforce **audience** when audience checking is enabled for the route
  (§4). When enabled, `aud` **MUST** contain your service's exact resource id.
- **MUST** return `401` (with `WWW-Authenticate`) for signature/`iss`/`exp`/
  unknown-`kid`/bad-`cls` failures, and `403` for *authenticated-but-not-allowed*
  (entitlement) failures. Never `5xx` for a bad token.
- **SHOULD** treat `sub` as the stable identity key for entitlement lookups and
  logging; **SHOULD** log `jti`, `cls`, tenant, and route for audit.
- **SHOULD NOT** use `rol` for fine-grained authorization — it is coarse and
  advisory; defer to the entitlement server.

---

## 3. Honouring user-based **and** app-based tokens (`cls`)

The single most important behavior: **both** principal kinds are first-class.
Your middleware must accept both and normalize them.

```mermaid
flowchart TD
    T[Verified token] --> C{cls?}
    C -- user --> U[principal.kind = user<br/>id = sub = email]
    C -- app --> A[principal.kind = app<br/>id = sub = client_id]
    C -- other/missing --> X[401 invalid_token]
    U --> TEN[tenant_id = parse rsc]
    A --> TEN
    TEN --> ENT[entitlement: groups/permissions for sub in tenant_id]
    ENT --> DEC{permitted?}
    DEC -- yes --> OK[allow]
    DEC -- no --> F[403 forbidden]
```

| `cls`  | `sub` is… | typical caller | how to treat it |
| ------ | --------- | -------------- | --------------- |
| `user` | email | a human (interactive login, auth-code, refresh) | act as that human within their tenant |
| `app`  | client_id | a machine app acting **for its owner** | act as that app's owner within the owner's tenant |

Both are tenant-bound identically (`rsc`). The entitlement decision is the same
shape — "which groups does `sub` belong to in this tenant?" — so most code paths
don't branch on `cls`; you branch only when a route is *human-only* or
*machine-only*.

> **Do not** assume `sub` is an email. For `cls=app` it is a `client_id`. Key
> entitlement and logging on `(cls, sub, tenant_id)`.

---

## 4. Opt-in audience check

Audience (`aud`) is the **audience-confusion guard**: it ensures a token minted
for service X cannot be replayed against service Y just because the signature is
valid. The identity server sets `aud` from the client's RFC 8707 `resource`
request (allowlist-gated) or a default audience.

Because not every service has a registered resource id yet (and some shared
tokens carry the default `aud`), audience enforcement is **opt-in per service /
per route**:

| Mode | When to use | Behavior |
| ---- | ----------- | -------- |
| **enforced** (recommended for resource servers) | your service has a stable resource id and clients request it via `resource=` | reject (`401`) unless `aud` contains your exact resource id |
| **off** (default-safe fallback) | early integration, or routes that legitimately accept default-audience tokens | skip the `aud` value check (signature/iss/exp still enforced) |

### 4.1 Configuration shape

```
AUTH_AUDIENCE_REQUIRED = true|false      # master switch (default false)
AUTH_RESOURCE_ID       = "https://grasp-daas.com/api/state/v1"   # this service's id
```

- When `AUTH_AUDIENCE_REQUIRED=true`: `AUTH_RESOURCE_ID` **must** be present in
  the token's `aud` (string or array) or the request is `401 invalid_token`
  (`error_description="audience"`).
- When `false`: do not validate the `aud` *value*, but still parse it for
  logging. **Flip to `true` once your clients request your resource id** — that
  is the secure end state.
- Per-route override: a route may force enforcement even if the global switch is
  off (e.g. a sensitive admin route).

> Coordinate with the IdP: your `AUTH_RESOURCE_ID` must be on
> `OAUTH_RESOURCE_ALLOWLIST` for the IdP to honor it in `resource=` requests
> (otherwise tokens fall back to the default audience and enforcement would
> reject them).

---

## 5. Reference implementations

### 5.1 Python (FastAPI / Starlette-style), `PyJWT` + `PyJWKClient`

```python
import jwt
from jwt import PyJWKClient, PyJWKClientError

ISSUER = "https://auth.grasp-daas.com"                      # per env
JWKS_URI = f"{ISSUER}/oauth/.well-known/jwks.json"
RESOURCE_ID = "https://grasp-daas.com/api/state/v1"          # this service
AUDIENCE_REQUIRED = True                                     # opt-in switch

_jwks = PyJWKClient(JWKS_URI, cache_keys=True, lifespan=300)  # resolves by kid


class Principal:
    def __init__(self, claims: dict):
        cls = claims.get("cls")
        if cls not in ("user", "app"):
            raise Unauthorized("invalid_token", "bad cls")
        self.kind = cls                       # "user" | "app"
        self.id = claims["sub"]               # email (user) | client_id (app)
        self.tenant_id = claims["rsc"].split(":", 1)[0]   # uuid before ':'
        self.roles = claims.get("rol", [])
        self.jti = claims.get("jti")
        self.claims = claims


def authenticate(token: str) -> Principal:
    try:
        signing_key = _jwks.get_signing_key_from_jwt(token)   # by kid
        # Verify signature + iss + exp always; verify aud only when opted in.
        decode_opts = {
            "algorithms": ["RS256"],            # RS256 only — never none/HS*
            "issuer": ISSUER,
            "leeway": 30,
            "options": {"require": ["exp", "iss", "sub"]},
        }
        if AUDIENCE_REQUIRED:
            decode_opts["audience"] = RESOURCE_ID            # enforce aud
        else:
            decode_opts["options"]["verify_aud"] = False     # parse, don't enforce
        claims = jwt.decode(token, signing_key.key, **decode_opts)
        return Principal(claims)
    except (PyJWKClientError, jwt.PyJWTError) as exc:
        raise Unauthorized("invalid_token", str(exc))          # 401, not 5xx


def authorize(principal: Principal, action: str) -> None:
    # Identity from the token; permission from the entitlement server.
    if not entitlement.allows(principal.tenant_id, principal.kind,
                              principal.id, action):
        raise Forbidden()                                      # 403
```

### 5.2 Node (Express-style), `jose`

```js
import { createRemoteJWKSet, jwtVerify } from "jose";

const ISSUER = "https://auth.grasp-daas.com";                 // per env
const RESOURCE_ID = "https://grasp-daas.com/api/state/v1";    // this service
const AUDIENCE_REQUIRED = true;                               // opt-in switch

const JWKS = createRemoteJWKSet(
  new URL(`${ISSUER}/oauth/.well-known/jwks.json`),
  { cacheMaxAge: 600_000, cooldownDuration: 30_000 }          // resolves by kid
);

export async function authenticate(token) {
  const opts = { issuer: ISSUER, algorithms: ["RS256"], clockTolerance: 30 };
  if (AUDIENCE_REQUIRED) opts.audience = RESOURCE_ID;          // enforce aud
  let payload;
  try {
    ({ payload } = await jwtVerify(token, JWKS, opts));
  } catch (e) {
    throw new Unauthorized("invalid_token", e.message);        // 401
  }
  if (payload.cls !== "user" && payload.cls !== "app")
    throw new Unauthorized("invalid_token", "bad cls");
  return {
    kind: payload.cls,                       // "user" | "app"
    id: payload.sub,                         // email | client_id
    tenantId: String(payload.rsc).split(":")[0],
    roles: payload.rol || [],
    jti: payload.jti,
    claims: payload,
  };
}
```

### 5.3 Framework-agnostic pseudocode

```text
fn middleware(request):
    token = bearer(request)                       or -> 401
    kid   = header(token).kid
    key   = jwks.get(kid)                          # rotation contract; -> 401 if unknown
    claims = rs256_verify(token, key)              # signature; -> 401
    assert claims.iss == ISSUER                    # -> 401
    assert not expired(claims, leeway=30s)         # -> 401
    if AUDIENCE_REQUIRED or route.requires_audience:
        assert RESOURCE_ID in as_list(claims.aud)  # -> 401 audience
    if claims.cls not in {"user","app"}:           # -> 401
    principal = {
        kind: claims.cls,
        id:   claims.sub,                          # email | client_id
        tenant_id: split(claims.rsc, ":")[0],
        roles: claims.rol,
    }
    request.principal = principal
    # authorization deferred to entitlement server, keyed by (tenant_id, kind, id)
```

---

## 6. Anti-patterns

- **Branching auth on `rol`** for fine-grained decisions. `rol` is coarse;
  permissions live in the entitlement server.
- **Assuming `sub` is an email.** It's a `client_id` for `cls=app`.
- **Ignoring `rsc`/tenant.** Every data path must be tenant-scoped; a valid
  token for tenant A must never read tenant B.
- **Leaving audience off forever.** "Off" is a migration state, not the end
  state. Register your resource id, get clients to request it, then enforce.
- **Rejecting both `cls` kinds with the same code path that only handles one.**
  Apps and users are both legitimate; design for both from day one.
- **Returning `5xx` for invalid tokens.** Bad token → `401`; authenticated but
  unauthorized → `403`.
- (See the rotation contract for key-handling anti-patterns: pinning keys,
  caching JWKS forever, assuming a single key, accepting `alg` from the token.)

---

## 7. Per-service checklist

- [ ] Verify RS256 by `kid` from live JWKS; survive rotation (rotation contract).
- [ ] Enforce `iss` and `exp`/`nbf` (≤ 60 s skew) always.
- [ ] Parse `cls`; accept **both** `user` and `app`; reject anything else (401).
- [ ] Normalize principal `{kind, id=sub, tenant_id=rsc.split(":")[0], roles}`.
- [ ] Scope all data/authz to `tenant_id`; cross-tenant impossible from token.
- [ ] Audience: register `AUTH_RESOURCE_ID` on the IdP allowlist, have clients
      request it, then set `AUTH_AUDIENCE_REQUIRED=true`.
- [ ] Defer permissions to the entitlement server keyed by `(tenant_id, kind, sub)`.
- [ ] `401` for bad token, `403` for not-allowed; never `5xx`.
- [ ] Log `jti`, `cls`, `sub`, `tenant_id`, route for audit.
