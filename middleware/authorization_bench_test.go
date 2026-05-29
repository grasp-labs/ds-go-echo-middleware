package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
)

// injectUniqueContext generates a fresh unique Sub on every request so the
// cache always misses, isolating the HTTP call cost in cache-miss benchmarks.
func injectUniqueContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userContext", &claims.Context{
				Sub: uuid.New().String(),
				Jti: uuid.New(),
				Rsc: uuid.New().String() + ":bench-tenant",
			})
			c.Set("Authorization", "Bearer fake-token")
			return next(c)
		}
	}
}

func newBenchEntitlementsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := []map[string]any{
			{"id": uuid.New().String(), "name": "required-group", "tenant_id": uuid.New().String()},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

func newBenchMiddlewareEcho(entitlementsURL string, contextMiddleware echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	cfg := fakes.NewConfig("dp", "core", "bench-svc", "v0.0.1", uuid.New(), 512)
	logger := &fakes.MockLogger{}
	producer := &adapters.ProducerAdapter{Producer: &fakes.MockProducer{}}
	e.Use(contextMiddleware)
	e.Use(middleware.AuthorizationMiddleware(cfg, logger, []string{"required-group"}, entitlementsURL, producer, "bench.topic", middleware.DefaultEntitlementTimeout))
	e.GET("/protected/", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

// BenchmarkCacheHit measures the hot path: entitlement already cached.
func BenchmarkCacheHit(b *testing.B) {
	srv := newBenchEntitlementsServer()
	defer srv.Close()

	fixedClaims := &claims.Context{Sub: "bench-user@example.com", Jti: uuid.New(), Rsc: uuid.New().String() + ":t"}
	e := newBenchMiddlewareEcho(srv.URL+"/groups/", injectContext(fixedClaims))

	// Warm the cache.
	warm := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	e.ServeHTTP(httptest.NewRecorder(), warm)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
		e.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// BenchmarkCacheMiss measures the cold path: every request hits the entitlement
// API because each request carries a unique Sub.
func BenchmarkCacheMiss(b *testing.B) {
	srv := newBenchEntitlementsServer()
	defer srv.Close()

	e := newBenchMiddlewareEcho(srv.URL+"/groups/", injectUniqueContext())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
		e.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// BenchmarkCacheMissParallel measures concurrent cache misses — the scenario
// where connection pooling has the most impact.
func BenchmarkCacheMissParallel(b *testing.B) {
	srv := newBenchEntitlementsServer()
	defer srv.Close()

	// Single shared Echo instance: all goroutines share the same http.Client.
	e := newBenchMiddlewareEcho(srv.URL+"/groups/", injectUniqueContext())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
			e.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
