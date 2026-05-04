package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
)

func newAuthzTestClaims() *claims.Context {
	return &claims.Context{
		Sub: "test-user@example.com",
		Jti: uuid.New(),
		Rsc: uuid.New().String() + ":test-tenant",
	}
}

func newAuthzEcho(t *testing.T, userClaims *claims.Context, entitlementsURL string) *echo.Echo {
	t.Helper()
	e := echo.New()

	cfg := fakes.NewConfig("dp", "core", "test-svc", "v0.0.1", uuid.New(), 512)
	logger := &fakes.MockLogger{}
	producer := &adapters.ProducerAdapter{Producer: &fakes.MockProducer{}}
	topic := "ds.test.authz.v1"

	e.Use(injectContext(userClaims))
	e.Use(middleware.AuthorizationMiddleware(cfg, logger, []string{"required-group"}, entitlementsURL, producer, topic))
	e.GET("/protected/", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

// injectContext returns a middleware that plants userContext and Authorization
// into the Echo context, mimicking what the authentication middleware does.
func injectContext(userClaims *claims.Context) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userContext", userClaims)
			c.Set("Authorization", "Bearer fake-token")
			return next(c)
		}
	}
}

// mockEntitlementsServer spins up a test HTTP server that returns the given
// groups as an entitlements response.
func mockEntitlementsServer(t *testing.T, groups []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := make([]map[string]any, len(groups))
		for i, g := range groups {
			payload[i] = map[string]any{
				"id":        uuid.New().String(),
				"name":      g,
				"tenant_id": uuid.New().String(),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

// --- Bypass tests ---

// TestAuthorizationMiddleware_AdminGroupBypass verifies that a user who is a
// member of the "admin" entitlements group is allowed through regardless of
// the required roles configured on the middleware.
func TestAuthorizationMiddleware_AdminGroupBypass(t *testing.T) {
	entitlementsCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entitlementsCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": uuid.New().String(), "name": "users.admins", "tenant_id": uuid.New().String()},
		})
	}))
	defer srv.Close()

	// Required role is "required-group"; user is only in "admin" — bypass should fire.
	e := newAuthzEcho(t, newAuthzTestClaims(), srv.URL+"/groups/")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, entitlementsCalled, "entitlements must still be called to discover admin membership")
}

// --- Normal-path tests (entitlements called) ---

// TestAuthorizationMiddleware_RegularRoleCallsEntitlements confirms that a JWT
// without "root" goes through the entitlements graph check and is granted
// access when the entitlements service returns a matching group.
func TestAuthorizationMiddleware_RegularRoleCallsEntitlements(t *testing.T) {
	entitlementsCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entitlementsCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": uuid.New().String(), "name": "required-group", "tenant_id": uuid.New().String()},
		})
	}))
	defer srv.Close()

	e := newAuthzEcho(t, newAuthzTestClaims(), srv.URL+"/groups/")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, entitlementsCalled, "entitlements service must be called for non-bypass roles")
}

// TestAuthorizationMiddleware_NonBypassRoleNotInGroup confirms that a non-bypass
// JWT whose subject is absent from the required entitlements group is denied.
func TestAuthorizationMiddleware_NonBypassRoleNotInGroup(t *testing.T) {
	srv := mockEntitlementsServer(t, []string{"some-other-group"})
	defer srv.Close()

	e := newAuthzEcho(t, newAuthzTestClaims(), srv.URL+"/groups/")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestAuthorizationMiddleware_EmptyRolesArray guards against an accidental
// "empty rol claim = bypass" regression: an empty Rol slice must still fall
// through to the entitlements check and be denied if not in the group.
func TestAuthorizationMiddleware_EmptyRolesArray(t *testing.T) {
	entitlementsCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entitlementsCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	e := newAuthzEcho(t, newAuthzTestClaims(), srv.URL+"/groups/")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.True(t, entitlementsCalled, "entitlements must still be called when Rol is empty")
}

// TestAuthorizationMiddleware_EntitlementsReturnsError confirms fail-closed
// behaviour: when the entitlements service returns a 5xx, the middleware denies
// the request rather than allowing it through.
func TestAuthorizationMiddleware_EntitlementsReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := newAuthzEcho(t, newAuthzTestClaims(), srv.URL+"/groups/")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Infrastructure tests ---

// TestAuthorizationMiddleware_MissingUserContext checks that a missing user
// context is denied (errorHandler always returns echo.ErrForbidden).
func TestAuthorizationMiddleware_MissingUserContext(t *testing.T) {
	e := echo.New()
	cfg := fakes.NewConfig("dp", "core", "test-svc", "v0.0.1", uuid.New(), 512)
	logger := &fakes.MockLogger{}
	producer := &adapters.ProducerAdapter{Producer: &fakes.MockProducer{}}
	topic := "ds.test.authz.v1"

	e.Use(middleware.AuthorizationMiddleware(cfg, logger, []string{"required-group"}, "http://unused/", producer, topic))
	e.GET("/protected/", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
