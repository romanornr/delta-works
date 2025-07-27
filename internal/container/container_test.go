package container

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/romanornr/delta-works/internal/contracts"
)

// mockLogger is a mock implementation of the Logger interface
type mockLogger struct {
	id int64
}

func (m *mockLogger) Info() contracts.LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Debug() contracts.LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Warn() contracts.LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Error() contracts.LogEvent {
	return &mockLogEvent{}
}

// mockLogEvent is a mock implementation of the LogEvent interface
type mockLogEvent struct{}

func (m *mockLogEvent) Msg(msg string) {}

func (m *mockLogEvent) Msgf(format string, a ...any) {}

func (m *mockLogEvent) Err(err error) contracts.LogEvent {
	return m
}

func (m *mockLogEvent) Str(key, value string) contracts.LogEvent {
	return m
}

func (m *mockLogEvent) Int(key string, value int) contracts.LogEvent {
	return m
}

// mockLoggerIDCounter is used to generate unique IDs for mockLogger instances.
// This counter works in conjunction with the id field in mockLogger to prevent
// Go's empty struct optimization from causing test failures.
var mockLoggerIDCounter int64

func TestServiceContainer_RegisterAndGet(t *testing.T) {
	logger := &mockLogger{}
	container := NewServiceContainer(logger)

	// Test SharedResource registration and retrieval
	serviceType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
	container.RegisterSharedResource(serviceType, func(c *ServiceContainer) (interface{}, error) {
		// Use atomic.AddInt64() to safely generate unique IDs across goroutines.
		// This ensures each mockLogger instance gets a unique id field value,
		// preventing Go's empty struct optimization from reusing memory addresses
		// and causing different instances to appear identical in tests.
		id := atomic.AddInt64(&mockLoggerIDCounter, 1)
		return &mockLogger{id: id}, nil
	})

	// Get the service
	service, err := container.Get(serviceType)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}

	if service == nil {
		t.Error("Service should not be nil")
	}

	// Verify it's the same instance (SharedResource behavior)
	service2, err := container.Get(serviceType)
	if err != nil {
		t.Fatalf("Failed to get the service the second time: %v", err)
	}

	if service != service2 {
		t.Errorf("SharedResource should return the same instance, got %v, want %v", service2, service)
	}
}

func TestServiceContainer_AlwaysNew(t *testing.T) {
	logger := &mockLogger{}
	container := NewServiceContainer(logger)

	serviceType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
	container.RegisterAlwaysNew(serviceType, func(c *ServiceContainer) (interface{}, error) {
		// Use atomic.AddInt64() to safely generate unique IDs across goroutines.
		// This ensures each mockLogger instance gets a unique id field value,
		// preventing Go's empty struct optimization from reusing memory addresses
		// and causing different instances to appear identical in tests.
		id := atomic.AddInt64(&mockLoggerIDCounter, 1)
		return &mockLogger{id: id}, nil
	})

	// Get the service
	service1, err := container.Get(serviceType)
	if err != nil {
		t.Fatalf("Failed to get the service: %v", err)
	}

	service2, err := container.Get(serviceType)
	if err != nil {
		t.Fatalf("Failed to get service second time: %v", err)
	}

	if service1 == service2 {
		t.Errorf("AlwaysNew should return different new instances")
	}
}

func TestServiceContainer_ScopedResource(t *testing.T) {
	logger := &mockLogger{}
	container := NewServiceContainer(logger)

	serviceType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
	container.RegisterScopedResource(serviceType, func(c *ServiceContainer) (interface{}, error) {
		// Use atomic.AddInt64() to safely generate unique IDs across goroutines.
		// This ensures each mockLogger instance gets a unique id field value,
		// preventing Go's empty struct optimization from reusing memory addresses
		// and causing different instances to appear identical in tests.
		id := atomic.AddInt64(&mockLoggerIDCounter, 1)
		return &mockLogger{id: id}, nil
	})

	scopeID := "test-scope"
	container.CreateScope(scopeID)
	defer container.DisposeScope(scopeID)

	// Get the service twice in the same scope
	service1, err := container.GetScoped(serviceType, scopeID)
	if err != nil {
		t.Fatalf("Failed to get scoped service: %v", err)
	}

	service2, err := container.GetScoped(serviceType, scopeID)
	if err != nil {
		t.Fatalf("Failed to get scoped service second time: %v", err)
	}

	if service1 != service2 {
		t.Errorf("ScopedResource should return same instance within scope")
	}

	// Create a different scope
	scopeID2 := "test-scope-2"
	container.CreateScope(scopeID2)
	defer container.DisposeScope(scopeID2)

	service3, err := container.GetScoped(serviceType, scopeID2)
	if err != nil {
		t.Fatalf("Failed to get scoped service in different scope %v", err)
	}

	fmt.Printf("service1: %p, service2: %p, service3: %p\n", service1, service2, service3)

	if service1 == service3 {
		t.Errorf("ScopedResource should return different instances in different scopes")
	}
}

func TestServiceContainer_Unregistered(t *testing.T) {
	logger := &mockLogger{}
	container := NewServiceContainer(logger)

	serviceType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
	_, err := container.Get(serviceType)
	if err == nil {
		t.Error("should return error for unregistered service")
	}
}

func TestServiceContainer_ConcurrentAccess(t *testing.T) {
	logger := &mockLogger{}
	container := NewServiceContainer(logger)

	serviceType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
	container.RegisterSharedResource(serviceType, func(c *ServiceContainer) (interface{}, error) {
		time.Sleep(10 * time.Millisecond)
		return &mockLogger{}, nil
	})

	var wg sync.WaitGroup
	results := make([]interface{}, 10)

	// Start 10 goroutines trying to get the same service
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			service, err := container.Get(serviceType)
			if err != nil {
				t.Errorf("Failed to get service: %v", err)
				return
			}
			results[index] = service
		}(i)
	}

	wg.Wait()

	// All results should be the same instance (SharedResource behavior)
	firstResult := results[0]
	for i, result := range results {
		if result != firstResult {
			t.Errorf("Expected same service instance %d, got %v, want %v", i, result, firstResult)
		}
	}
}

func TestServiceContainer_ScopeDisposal(t *testing.T) {
	logger := &mockLogger{}
	container := NewServiceContainer(logger)

	scopeID := "disposal-test"
	container.CreateScope(scopeID)

	container.mu.RLock()
	_, exists := container.scopes[scopeID]
	container.mu.RUnlock()

	if !exists {
		t.Error("Scope should exist after creation")
	}

	err := container.DisposeScope(scopeID)
	if err != nil {
		t.Error("Scope should exist after creation")
	}

	// verify scope is removed
	container.mu.RLock()
	_, exists = container.scopes[scopeID]
	container.mu.RUnlock()

	if exists {
		t.Error("Scope should not exist after disposal")
	}

}
