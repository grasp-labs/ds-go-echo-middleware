package fakes

import "context"

// --- Mock Producer ---
type MockProducer struct {
	called bool
	key    string
	value  any
}

func (m *MockProducer) Called() bool { return m.called }
func (m *MockProducer) Key() string  { return m.key }
func (m *MockProducer) Value() any   { return m.value }

func (p *MockProducer) Close() error {
	return nil
}

func (m *MockProducer) Send(ctx context.Context, key string, value any) error {
	m.called = true
	m.key = key
	m.value = value
	return nil
}
