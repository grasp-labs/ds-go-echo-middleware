package middleware

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// JWKS cache tuning (per the key-rotation contract §3). These are deliberately
// far shorter than the IdP's rotation overlap window (~1 day), so correctness
// never depends on the TTL.
const (
	jwksDefaultTTL      = 5 * time.Minute
	jwksDefaultCooldown = 30 * time.Second
	jwksHTTPTimeout     = 5 * time.Second
)

// jwk is a single RSA key entry from a JWKS document.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

// jwksCache resolves RSA verifying keys by `kid` from a remote JWKS, caching the
// set with a TTL and refreshing on an unknown `kid` (cooldown-gated). On a fetch
// failure it serves the last good key set rather than failing closed.
type jwksCache struct {
	uri      string
	ttl      time.Duration
	cooldown time.Duration
	client   *http.Client

	// refreshMu serializes network refreshes so only one goroutine fetches at a
	// time; mu guards the in-memory key set. Cache hits take only mu.
	refreshMu sync.Mutex

	mu         sync.Mutex
	keysByKid  map[string]*rsa.PublicKey
	fetchedAt  time.Time
	lastFetch  time.Time
	hasFetched bool
}

func newJWKSCache(uri string) *jwksCache {
	return &jwksCache{
		uri:       uri,
		ttl:       jwksDefaultTTL,
		cooldown:  jwksDefaultCooldown,
		client:    &http.Client{Timeout: jwksHTTPTimeout},
		keysByKid: map[string]*rsa.PublicKey{},
	}
}

// getKey returns the RSA public key for kid, refreshing the JWKS if the kid is
// unknown or the cache is stale (subject to the cooldown). Returns an error when
// the kid cannot be resolved — callers MUST treat that as a 401, never a 5xx.
//
// Cache hits take only the in-memory lock and never wait on the network. When a
// refresh is needed, refreshMu serializes it so a burst of misses triggers a
// single fetch that all waiters share, rather than stampeding the IdP or
// stalling on a lock held across the HTTP call.
func (j *jwksCache) getKey(kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("token missing kid header")
	}

	// Fast path: a fresh, known key needs no refresh.
	if key, ok := j.lookup(kid, true); ok {
		return key, nil
	}

	j.refreshMu.Lock()
	defer j.refreshMu.Unlock()

	// Another goroutine may have refreshed while we waited for refreshMu.
	if key, ok := j.lookup(kid, true); ok {
		return key, nil
	}

	// Cooldown gate: refresh at most once per cooldown, even on failure.
	j.mu.Lock()
	eligible := !j.hasFetched || time.Since(j.lastFetch) >= j.cooldown
	if eligible {
		j.lastFetch = time.Now()
	}
	j.mu.Unlock()

	if eligible {
		// On failure (or empty set) keep the last-good keys: serve stale.
		if keys, err := j.fetch(); err == nil && len(keys) > 0 {
			j.mu.Lock()
			j.keysByKid = keys
			j.fetchedAt = time.Now()
			j.hasFetched = true
			j.mu.Unlock()
		}
	}

	// Return whatever we now have (possibly stale); unknown kid => error => 401.
	if key, ok := j.lookup(kid, false); ok {
		return key, nil
	}
	return nil, fmt.Errorf("no key for kid %q in JWKS", kid)
}

// lookup returns the key for kid. When requireFresh is set, it only returns a
// hit if the cached set is still within its TTL.
func (j *jwksCache) lookup(kid string, requireFresh bool) (*rsa.PublicKey, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	key, ok := j.keysByKid[kid]
	if !ok {
		return nil, false
	}
	if requireFresh && !(j.hasFetched && time.Since(j.fetchedAt) < j.ttl) {
		return nil, false
	}
	return key, true
}

func (j *jwksCache) fetch() (map[string]*rsa.PublicKey, error) {
	resp, err := j.client.Get(j.uri)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks fetch returned status %d", resp.StatusCode)
	}

	var doc jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}

	out := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		// Only RSA keys are relevant; the set may also carry the OIDC id_token
		// key or non-RSA keys — skip anything we can't use.
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := jwkToRSAPublicKey(k)
		if err != nil {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("jwks contained no usable RSA keys")
	}
	return out, nil
}

func jwkToRSAPublicKey(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}

	// Right-align the exponent into 8 bytes to read it as a big-endian uint64.
	if len(eBytes) > 8 {
		return nil, errors.New("exponent too large")
	}
	var eBuf [8]byte
	copy(eBuf[8-len(eBytes):], eBytes)
	e := binary.BigEndian.Uint64(eBuf[:])
	if e == 0 {
		return nil, errors.New("invalid zero exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e),
	}, nil
}
