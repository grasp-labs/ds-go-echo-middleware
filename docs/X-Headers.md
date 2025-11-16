# ✅ Supported Headers

## ⚙️ X-Headers (custom / compatibility)

| Header           | Purpose                                                                                                | Recommendation                                                                                          |
| ---------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| **X-Request-ID** | *Optional client-supplied unique ID used for audit, usage, and AuthX/AuthY trails.*                    | Accept if valid UUID; otherwise generate one server-side and echo back. Prefer `traceparent` long-term. |
| **X-Session-ID** | *Optional client-supplied session ID to bind multiple related requests.*                               | Accept only from trusted clients; otherwise generate ephemeral session ID for telemetry correlation.    |
| **X-Owner-ID**   | *Optional client-supplied identifier used for cost attribution or ownership tracking.*                 | Treat as metadata, not authorization. Validate format and origin (e.g., internal header only).          |
| **X-Message**    | *Optional client-supplied message or context string included in audit, usage, and AuthX/AuthY trails.* | Sanitize before logging. Keep short (<1 KB) to avoid log spam or injection.                             |

🧩 Note: These are “legacy custom” headers, not IANA-standard. Continue supporting them for compatibility, but consider migrating toward standardized fields or structured metadata in the request body for future versions.

## 📦 Standard Headers (actively used)

| Header                           | Purpose                                                                                                      | Typical Usage / Managed By                                                                                              |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| **Content-Type**                 | Defines request/response media type.                                                                         | Set automatically by Echo and framework responses.                                                                      |
| **Accept-Language**              | Indicates client locale and language preferences.                                                            | Used by the `locale` middleware for translation/localization.                                                           |
| **Retry-After**                  | Tells clients when to retry after rate limit or temporary errors (429 / 503).                                | Set by `commonmodels.http_error`.                                                                                       |
| **ETag**                         | Unique quoted identifier for a resource version (e.g., `"abc123"`).                                          | Returned by File APIs and cacheable endpoints. Used with `If-Match` and `If-None-Match` for version checks.             |
| **If-Match**                     | Ensures the resource hasn’t changed since the last retrieval (`ETag` match required).                        | Used for **optimistic concurrency control** (update/delete). Respond with `412 Precondition Failed` if ETag mismatched. |
| **If-None-Match**                | Validates that the resource hasn’t changed; used to return `304 Not Modified` if cached copy is still valid. | Used by File and cacheable APIs for conditional GET.                                                                    |
| **Last-Modified**                | Timestamp of last resource update.                                                                           | Set by File APIs; used with `If-Modified-Since` or `If-Unmodified-Since` for conditional requests.                      |

## Future / planned

| Header                                    | Purpose                                                  | Notes                                                                                               |
| ----------------------------------------- | -------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| **X-RateLimit-Limit / Remaining / Reset** | Provide clients with usage limits and window reset time. | Good candidate for migration to RFC 9444 `RateLimit-Limit / Remaining / Reset` (standardized form). |
| **Cache-Control** *(optional)*   | Controls caching behavior for clients and intermediaries.                                                    | Recommended for all file-serving endpoints.                                                                             |
| **Idempotency-Key** *(optional)* | Allows safe retries for POST requests without duplication.                                                   | Useful for write-heavy APIs; consider adding in future releases.                                                        |
