package middleware_test

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
)

// tokenOpts controls minting of a test JWT.
type tokenOpts struct {
	iss string
	cls string // "" omits the claim
	kid string // "" omits the header
	sub string
	rsc string
	aud []string
	exp time.Time
	nbf time.Time
	iat time.Time
}

func mintToken(t *testing.T, priv *rsa.PrivateKey, o tokenOpts) string {
	t.Helper()
	now := time.Now()
	if o.iss == "" {
		o.iss = testIssuer
	}
	if o.sub == "" {
		o.sub = "test-user@example.com"
	}
	if o.rsc == "" {
		o.rsc = uuid.New().String() + ":test-tenant"
	}
	if o.exp.IsZero() {
		o.exp = now.Add(time.Hour)
	}
	if o.nbf.IsZero() {
		o.nbf = now.Add(-time.Minute)
	}
	if o.iat.IsZero() {
		o.iat = now.Add(-time.Minute)
	}

	claims := jwt.MapClaims{
		"iss": o.iss,
		"sub": o.sub,
		"rsc": o.rsc,
		"aud": o.aud,
		"exp": float64(o.exp.Unix()),
		"nbf": float64(o.nbf.Unix()),
		"iat": float64(o.iat.Unix()),
		"jti": uuid.New().String(),
	}
	if o.cls != "" {
		claims["cls"] = o.cls
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if o.kid != "" {
		tok.Header["kid"] = o.kid
	}
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)
	return signed
}

// newAuthApp builds an echo instance behind AuthenticationMiddleware with the
// given issuer and options, exposing /me (echoes the principal) and /protected.
func newAuthApp(t *testing.T, pubPEM, issuer string, opts ...middleware.AuthOption) *echo.Echo {
	t.Helper()
	e := echo.New()
	cfg := fakes.NewConfig("dp", "core", "test-svc", "v0.0.1", uuid.New(), 512)
	cfg.SetIssuer(issuer)
	logger := &fakes.MockLogger{}
	producer := &adapters.ProducerAdapter{Producer: &fakes.MockProducer{}}

	authMW, err := middleware.AuthenticationMiddleware(cfg, logger, pubPEM, producer, "ds.test.v1", opts...)
	require.NoError(t, err)
	e.Use(authMW)

	e.GET("/protected/", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/me", func(c echo.Context) error {
		p, ok := requestctx.GetPrincipal(c.Request().Context())
		if !ok {
			return c.NoContent(http.StatusInternalServerError)
		}
		return c.JSON(http.StatusOK, map[string]string{
			"kind":   p.Kind,
			"id":     p.ID,
			"tenant": p.TenantID.String(),
		})
	})
	return e
}

func doGet(t *testing.T, e *echo.Echo, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// ---------- cls ----------

func TestAuthN_AcceptsUserAndApp(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	for _, cls := range []string{"user", "app"} {
		tok := mintToken(t, priv, tokenOpts{cls: cls})
		rec := doGet(t, e, "/protected/", tok)
		assert.Equal(t, http.StatusOK, rec.Code, "cls=%s should be accepted", cls)
	}
}

func TestAuthN_RejectsUnknownCls(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	tok := mintToken(t, priv, tokenOpts{cls: "robot"})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthN_RejectsMissingCls(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	tok := mintToken(t, priv, tokenOpts{cls: ""}) // no cls claim
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- issuer ----------

func TestAuthN_EnforcesEnvIssuer(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	// Service configured for prod issuer; token minted by dev issuer → reject.
	e := newAuthApp(t, pubPEM, "https://auth.grasp-daas.com")

	tok := mintToken(t, priv, tokenOpts{iss: "https://auth-dev.grasp-daas.com", cls: "user"})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- leeway ----------

func TestAuthN_LeewayToleratesSmallSkew(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	// Expired 10s ago — within the 30s leeway → still accepted.
	tok := mintToken(t, priv, tokenOpts{cls: "user", exp: time.Now().Add(-10 * time.Second)})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthN_RejectsClearlyExpired(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	tok := mintToken(t, priv, tokenOpts{cls: "user", exp: time.Now().Add(-5 * time.Minute)})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- principal ----------

func TestAuthN_PrincipalOnContext(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	tenant := uuid.New()
	tok := mintToken(t, priv, tokenOpts{
		cls: "app",
		sub: "client-123",
		rsc: tenant.String() + ":acme",
	})
	rec := doGet(t, e, "/me", tok)
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "app", body["kind"])
	assert.Equal(t, "client-123", body["id"])
	assert.Equal(t, tenant.String(), body["tenant"])
}

// ---------- WWW-Authenticate ----------

func TestAuthN_401CarriesBearerChallenge(t *testing.T) {
	_, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer)

	rec := doGet(t, e, "/protected/", "") // no token
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
}

func TestAuthN_401ChallengePointsAtResourceMetadataWhenAudienceSet(t *testing.T) {
	_, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer, middleware.WithAudience(testResource))

	rec := doGet(t, e, "/protected/", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	want := `Bearer resource_metadata="` + testResource + middleware.WellKnownProtectedResourcePath + `"`
	assert.Equal(t, want, rec.Header().Get("WWW-Authenticate"))
}

// ---------- aud as a single string (RFC 7519 §4.1.3) ----------

func TestAuthN_AcceptsStringAudience(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer, middleware.WithAudience(testResource))

	now := time.Now()
	// aud is a single JSON string, not an array — must still parse and match.
	claims := jwt.MapClaims{
		"iss": testIssuer,
		"sub": "u@example.com",
		"cls": "user",
		"aud": testResource, // string, not []string
		"exp": float64(now.Add(time.Hour).Unix()),
		"nbf": float64(now.Add(-time.Minute).Unix()),
		"iat": float64(now.Add(-time.Minute).Unix()),
		"rsc": uuid.New().String() + ":tenant",
		"jti": uuid.New().String(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)

	rec := doGet(t, e, "/protected/", signed)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------- shared (mesh-wide) audience ----------

const testSharedAudience = "https://grasp-daas.com"

func TestAuthN_AcceptsSharedAudience(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer,
		middleware.WithAudience(testResource),
		middleware.WithSharedAudience(testSharedAudience),
	)

	// A default/mesh token carries only the shared host in aud.
	tok := mintToken(t, priv, tokenOpts{cls: "user", aud: []string{testSharedAudience}})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthN_AcceptsOwnResourceWhenSharedConfigured(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer,
		middleware.WithAudience(testResource),
		middleware.WithSharedAudience(testSharedAudience),
	)

	// A narrowed token carries this service's own resource id.
	tok := mintToken(t, priv, tokenOpts{cls: "user", aud: []string{testResource}})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthN_RejectsUnrelatedAudienceWhenSharedConfigured(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	e := newAuthApp(t, pubPEM, testIssuer,
		middleware.WithAudience(testResource),
		middleware.WithSharedAudience(testSharedAudience),
	)

	tok := mintToken(t, priv, tokenOpts{cls: "user", aud: []string{"https://grasp-daas.com/api/other/v1"}})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthN_SharedAudienceRejectedWhenOnlyResourceConfigured(t *testing.T) {
	priv, pubPEM, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)
	// No WithSharedAudience: a mesh token must NOT be accepted by a service that
	// only declared its own resource id.
	e := newAuthApp(t, pubPEM, testIssuer, middleware.WithAudience(testResource))

	tok := mintToken(t, priv, tokenOpts{cls: "user", aud: []string{testSharedAudience}})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- JWKS (rotation-safe) ----------

func jwksHandlerFor(keys map[string]*rsa.PublicKey) http.HandlerFunc {
	type jwkOut struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		out := struct {
			Keys []jwkOut `json:"keys"`
		}{}
		for kid, pub := range keys {
			out.Keys = append(out.Keys, jwkOut{
				Kty: "RSA", Kid: kid, Use: "sig", Alg: "RS256",
				N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

func TestAuthN_JWKS_VerifiesByKid(t *testing.T) {
	priv, _, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/.well-known/jwks.json", jwksHandlerFor(map[string]*rsa.PublicKey{
		"k1": &priv.PublicKey,
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// issuer == server base; middleware derives the JWKS URI from it.
	e := newAuthApp(t, "", srv.URL, middleware.WithJWKS())

	tok := mintToken(t, priv, tokenOpts{iss: srv.URL, cls: "user", kid: "k1"})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthN_JWKS_UnknownKidRejected(t *testing.T) {
	priv, _, err := fakes.GenerateRSAPairPEM()
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/.well-known/jwks.json", jwksHandlerFor(map[string]*rsa.PublicKey{
		"k1": &priv.PublicKey,
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := newAuthApp(t, "", srv.URL, middleware.WithJWKS())

	// Token references a kid the JWKS does not publish → 401, not 5xx.
	tok := mintToken(t, priv, tokenOpts{iss: srv.URL, cls: "user", kid: "unknown"})
	rec := doGet(t, e, "/protected/", tok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
