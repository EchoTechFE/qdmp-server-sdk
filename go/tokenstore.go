package qdmp

import "sync"

// TokenEntry is a cached app-level access token plus its absolute Unix-second
// expiry timestamp. This mirrors the wire format confirmed by real traffic
// capture: expiresAt is an absolute Unix seconds timestamp, not a relative
// "seconds remaining" duration and not milliseconds.
type TokenEntry struct {
	AccessToken string
	ExpiresAt   int64
}

// TokenStore is the pluggable persistence interface for the SDK's app-level
// (CLIENT_CREDENTIALS) token cache. The default implementation is an
// in-process memory store; multi-instance deployments can supply their own
// (e.g. backed by Redis) so that all instances share one cached token instead
// of each independently exchanging one and colliding/rate-limiting each
// other against the real qdmp gateway.
//
// User-level tokens (from AUTHORIZATION_CODE) are never stored here — the
// SDK never caches them, per the token lifecycle model in the design.
type TokenStore interface {
	Get() (TokenEntry, bool)
	Set(entry TokenEntry)
	Clear()
}

// memoryTokenStore is the default in-process TokenStore implementation.
type memoryTokenStore struct {
	mu    sync.RWMutex
	entry TokenEntry
	has   bool
}

// NewMemoryTokenStore returns a TokenStore backed by process memory. This is
// the default used by NewClient when ClientOptions.TokenStore is nil.
func NewMemoryTokenStore() TokenStore {
	return &memoryTokenStore{}
}

func (m *memoryTokenStore) Get() (TokenEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entry, m.has
}

func (m *memoryTokenStore) Set(entry TokenEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry = entry
	m.has = true
}

func (m *memoryTokenStore) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry = TokenEntry{}
	m.has = false
}
