package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// jwksTestServer is a controllable JWKS endpoint: its key set can be swapped to
// simulate rotation, request counts are tracked, and it can be made to fail.
type jwksTestServer struct {
	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	hits     int
	failwith int // when != 0, respond with this status
	srv      *httptest.Server
}

func newJWKSTestServer() *jwksTestServer {
	ts := &jwksTestServer{keys: map[string]*rsa.PublicKey{}}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		ts.hits++
		if ts.failwith != 0 {
			w.WriteHeader(ts.failwith)
			return
		}
		out := jwksDocument{}
		for kid, pub := range ts.keys {
			out.Keys = append(out.Keys, rsaToJWK(kid, pub))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	return ts
}

func (ts *jwksTestServer) setKeys(keys map[string]*rsa.PublicKey) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.keys = keys
}

func (ts *jwksTestServer) setFail(status int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.failwith = status
}

func (ts *jwksTestServer) hitCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.hits
}

func rsaToJWK(kid string, pub *rsa.PublicKey) jwk {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	return jwk{Kty: "RSA", Kid: kid, Use: "sig", Alg: "RS256", N: n, E: e}
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestJWKSCache_ResolvesByKid(t *testing.T) {
	k1 := mustRSAKey(t)
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey})

	cache := newJWKSCache(ts.srv.URL)
	got, err := cache.getKey("k1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.N.Cmp(k1.PublicKey.N) != 0 {
		t.Fatal("resolved key does not match k1")
	}
}

func TestJWKSCache_UnknownKidReturnsError(t *testing.T) {
	k1 := mustRSAKey(t)
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey})

	cache := newJWKSCache(ts.srv.URL)
	if _, err := cache.getKey("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown kid")
	}
}

func TestJWKSCache_PicksUpRotatedKey(t *testing.T) {
	k1 := mustRSAKey(t)
	k2 := mustRSAKey(t)
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey})

	cache := newJWKSCache(ts.srv.URL)
	cache.cooldown = 0 // allow immediate refresh on unknown kid

	if _, err := cache.getKey("k1"); err != nil {
		t.Fatalf("k1 should resolve: %v", err)
	}

	// Rotation: server now publishes both k1 (retiring) and k2 (active).
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey, "k2": &k2.PublicKey})

	got, err := cache.getKey("k2")
	if err != nil {
		t.Fatalf("k2 should resolve after refresh: %v", err)
	}
	if got.N.Cmp(k2.PublicKey.N) != 0 {
		t.Fatal("resolved key does not match k2")
	}
}

func TestJWKSCache_CooldownGatesRefresh(t *testing.T) {
	k1 := mustRSAKey(t)
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey})

	cache := newJWKSCache(ts.srv.URL)
	cache.cooldown = time.Hour // effectively block re-fetch

	if _, err := cache.getKey("k1"); err != nil {
		t.Fatalf("k1 should resolve: %v", err)
	}
	hitsAfterFirst := ts.hitCount()

	// Unknown kid within the cooldown must NOT trigger another fetch.
	if _, err := cache.getKey("k2"); err == nil {
		t.Fatal("expected error for unknown kid within cooldown")
	}
	if ts.hitCount() != hitsAfterFirst {
		t.Fatalf("cooldown should have prevented refetch: hits %d -> %d", hitsAfterFirst, ts.hitCount())
	}
}

func TestJWKSCache_ServesStaleOnFetchFailure(t *testing.T) {
	k1 := mustRSAKey(t)
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey})

	cache := newJWKSCache(ts.srv.URL)
	cache.ttl = 0      // force every lookup to consider the cache stale
	cache.cooldown = 0 // and allow a refresh attempt each time

	if _, err := cache.getKey("k1"); err != nil {
		t.Fatalf("k1 should resolve initially: %v", err)
	}

	// jwks_uri now fails; the last-good key must still resolve.
	ts.setFail(http.StatusInternalServerError)
	got, err := cache.getKey("k1")
	if err != nil {
		t.Fatalf("expected last-good key to be served on fetch failure: %v", err)
	}
	if got.N.Cmp(k1.PublicKey.N) != 0 {
		t.Fatal("stale-served key does not match k1")
	}
}

func TestJWKSCache_ConcurrentColdStartSingleFetch(t *testing.T) {
	k1 := mustRSAKey(t)
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	ts.setKeys(map[string]*rsa.PublicKey{"k1": &k1.PublicKey})

	cache := newJWKSCache(ts.srv.URL) // default 30s cooldown

	const n = 50
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = cache.getKey("k1")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d got error: %v", i, err)
		}
	}
	// A burst of concurrent cold-start misses must collapse to a single fetch.
	if got := ts.hitCount(); got != 1 {
		t.Fatalf("expected exactly 1 JWKS fetch under concurrency, got %d", got)
	}
}

func TestJWKSCache_MissingKidErrors(t *testing.T) {
	ts := newJWKSTestServer()
	defer ts.srv.Close()
	cache := newJWKSCache(ts.srv.URL)
	if _, err := cache.getKey(""); err == nil {
		t.Fatal("expected error for empty kid")
	}
}
