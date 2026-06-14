package middleware_test

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
)

const testResource = "https://grasp-daas.com/api/test-svc/v1"
const testIssuer = "https://auth.grasp-daas.com"

// ---------- helpers ----------

func newPRMMeta() middleware.ResourceMetadata {
	return middleware.ResourceMetadata{
		Resource:             testResource,
		AuthorizationServers: []string{testIssuer},
		ScopesSupported:      []string{"read", "write"},
	}
}

// signToken mints a signed RS256 JWT with the given aud.
func signToken(t *testing.T, priv *rsa.PrivateKey, aud []string) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": testIssuer,
		"sub": "test-user",
		"cls": "user",
		"aud": aud,
		"exp": float64(now.Add(time.Hour).Unix()),
		"nbf": float64(now.Add(-time.Minute).Unix()),
		"iat": float64(now.Add(-time.Minute).Unix()),
		"rsc": uuid.New().String() + ":test-tenant",
		"jti": uuid.New().String(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)
	return signed
}

// ---------- PRM handler tests ----------

func TestProtectedResourceHandler_200AndFields(t *testing.T) {
	e := echo.New()
	meta := newPRMMeta()
	middleware.RegisterProtectedResource(e, "/api/test-svc/v1", meta)

	req := httptest.NewRequest(http.MethodGet, "/api/test-svc/v1"+middleware.WellKnownProtectedResourcePath, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, meta.Resource, body["resource"])
	assert.Equal(t, []any{testIssuer}, body["authorization_servers"])
	assert.Equal(t, []any{"header"}, body["bearer_methods_supported"])
	assert.Equal(t, []any{"read", "write"}, body["scopes_supported"])
}

func TestProtectedResourceHandler_CORSAndCacheHeaders(t *testing.T) {
	e := echo.New()
	middleware.RegisterProtectedResource(e, "/api/test-svc/v1", newPRMMeta())

	req := httptest.NewRequest(http.MethodGet, "/api/test-svc/v1"+middleware.WellKnownProtectedResourcePath, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
}

func TestProtectedResourceHandler_NoAuthRequired(t *testing.T) {
	e := echo.New()
	meta := newPRMMeta()
	middleware.RegisterProtectedResource(e, "/api/test-svc/v1", meta)

	// Request without any Authorization header must still return 200.
	req := httptest.NewRequest(http.MethodGet, "/api/test-svc/v1"+middleware.WellKnownProtectedResourcePath, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------- 401 challenge tests ----------

func TestRegisterProtectedResource_401CarriesChallenge(t *testing.T) {
	e := echo.New()
	middleware.RegisterProtectedResource(e, "/api/test-svc/v1", newPRMMeta())

	e.GET("/protected/", func(c echo.Context) error {
		return echo.ErrUnauthorized
	})

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	want := `Bearer resource_metadata="` + testResource + middleware.WellKnownProtectedResourcePath + `"`
	assert.Equal(t, want, rec.Header().Get("WWW-Authenticate"))
}

func TestRegisterProtectedResource_Non401HasNoChallenge(t *testing.T) {
	e := echo.New()
	middleware.RegisterProtectedResource(e, "/api/test-svc/v1", newPRMMeta())

	e.GET("/missing/", func(c echo.Context) error {
		return echo.ErrNotFound
	})

	req := httptest.NewRequest(http.MethodGet, "/missing/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, rec.Header().Get("WWW-Authenticate"))
}

func TestRegisterProtectedResource_PriorErrorHandlerInvoked(t *testing.T) {
	e := echo.New()
	priorCalled := false
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		priorCalled = true
		_ = c.String(http.StatusUnauthorized, "prior handler")
	}

	middleware.RegisterProtectedResource(e, "/api/test-svc/v1", newPRMMeta())

	e.GET("/protected/", func(c echo.Context) error {
		return echo.ErrUnauthorized
	})

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, priorCalled, "prior HTTPErrorHandler must still be called")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- WithAudience tests ----------

func newAuthNEcho(t *testing.T, privKey *rsa.PrivateKey, pubPEM string, opts ...middleware.AuthOption) *echo.Echo {
	t.Helper()
	e := echo.New()
	cfg := fakes.NewConfig("dp", "core", "test-svc", "v0.0.1", uuid.New(), 512)
	logger := &fakes.MockLogger{}
	producer := &adapters.ProducerAdapter{Producer: &fakes.MockProducer{}}

	authMW, err := middleware.AuthenticationMiddleware(cfg, logger, pubPEM, producer, "ds.test.v1", opts...)
	require.NoError(t, err)
	e.Use(authMW)
	e.GET("/protected/", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

func TestWithAudience_MatchingAud_Passes(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	e := newAuthNEcho(t, priv, pubPEM, middleware.WithAudience(testResource))

	token := signToken(t, priv, []string{testResource})
	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWithAudience_AudContainsResourceAmongOthers_Passes(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	e := newAuthNEcho(t, priv, pubPEM, middleware.WithAudience(testResource))

	token := signToken(t, priv, []string{"https://other.grasp-daas.com/api/other/v1", testResource})
	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWithAudience_MissingAud_Returns401(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	e := newAuthNEcho(t, priv, pubPEM, middleware.WithAudience(testResource))

	token := signToken(t, priv, []string{}) // no aud
	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWithAudience_MismatchedAud_Returns401(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	e := newAuthNEcho(t, priv, pubPEM, middleware.WithAudience(testResource))

	token := signToken(t, priv, []string{"https://grasp-daas.com/api/other-service/v1"})
	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWithAudience_OptionAbsent_NoAudCheck(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	// No WithAudience option — token without matching aud must still pass.
	e := newAuthNEcho(t, priv, pubPEM)

	token := signToken(t, priv, []string{"https://grasp-daas.com/api/completely-different/v1"})
	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
