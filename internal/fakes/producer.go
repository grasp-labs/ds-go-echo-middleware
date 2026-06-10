package fakes

import (
	"context"
	"sync"
	"time"
)

// MockProducer is a thread-safe test double for adapters.Producer.
// Call WaitForSend before asserting Called()/Value() in tests that use
// sendEventAsync, since the real Send now runs in a goroutine.
type MockProducer struct {
	mu    sync.Mutex
	once  sync.Once
	ready chan struct{}

	called bool
	key    string
	value  any
}

func (m *MockProducer) ch() chan struct{} {
	m.once.Do(func() { m.ready = make(chan struct{}, 1) })
	return m.ready
}

// WaitForSend blocks until Send is called or the timeout elapses.
// Returns true if Send was (or had already been) called.
func (m *MockProducer) WaitForSend(timeout time.Duration) bool {
	select {
	case <-m.ch():
		return true
	case <-time.After(timeout):
		return false
	}
}

func (m *MockProducer) Called() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

func (m *MockProducer) Key() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.key
}

func (m *MockProducer) Value() any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.value
}

func (m *MockProducer) Close() error { return nil }

func (m *MockProducer) Send(ctx context.Context, key string, value any) error {
	m.mu.Lock()
	m.called = true
	m.key = key
	m.value = value
	m.mu.Unlock()

	select {
	case m.ch() <- struct{}{}:
	default:
	}
	return nil
}
