// Package container provides a dependency injection container with support for different instance strategies and scope management.
package container

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/romanornr/delta-works/internal/contracts"
)

type ServiceContainer struct {
	descriptors map[reflect.Type]*ServiceDescriptor
	scopes      map[string]map[reflect.Type]interface{}
	mu          sync.RWMutex
	logger      contracts.Logger
}

type ServiceDescriptor struct {
	// Factory function to create service instance
	Factory ServiceFactory

	// InstanceStrategy defines how service instances are managed
	Strategy InstanceStrategy

	// SharedInstance for SharedResource strategy
	SharedInstance interface{}

	// Dependencies this service requires
	Dependencies []reflect.Type

	// Whether this service has/needs a lifecycle
	HasLifeCycle bool
}

// InstanceStrategy defines how service instances are managed
type InstanceStrategy int

const (
	AlwaysNew InstanceStrategy = iota

	// ScopedResource creates one instance per scope (request-specific state)
	ScopedResource

	// SharedResource reuses expensive resources (e.g. database connections, loggers)
	SharedResource
)

type ServiceFactory func(container *ServiceContainer) (interface{}, error)

func NewServiceContainer(logger contracts.Logger) *ServiceContainer {
	return &ServiceContainer{
		descriptors: make(map[reflect.Type]*ServiceDescriptor),
		scopes:      make(map[string]map[reflect.Type]interface{}),
		logger:      logger,
	}
}

// RegisterAlwaysNew registers a service that creates a new instance every time
func (sc *ServiceContainer) RegisterAlwaysNew(serviceType reflect.Type, factory ServiceFactory) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.descriptors[serviceType] = &ServiceDescriptor{
		Factory:  factory,
		Strategy: AlwaysNew,
	}
}

// RegisterScopedResource registers a service that creates one instance per scope
func (sc *ServiceContainer) RegisterScopedResource(serviceType reflect.Type, factory ServiceFactory) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.descriptors[serviceType] = &ServiceDescriptor{
		Factory:  factory,
		Strategy: ScopedResource,
	}
}

// RegisterSharedResource registers a service that reuses expensive resources
func (sc *ServiceContainer) RegisterSharedResource(serviceType reflect.Type, factory ServiceFactory) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.descriptors[serviceType] = &ServiceDescriptor{
		Factory:  factory,
		Strategy: SharedResource,
	}
}

func (sc *ServiceContainer) Get(serviceType reflect.Type) (interface{}, error) {
	sc.mu.RLock()
	descriptor, exists := sc.descriptors[serviceType]
	sc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("service %s not found", serviceType)
	}

	switch descriptor.Strategy {
	case SharedResource:
		return sc.getSharedInstance(descriptor)
	case AlwaysNew:
		return descriptor.Factory(sc)
	default:
		return nil, fmt.Errorf("Get() can only be used with SharedResource or AlwaysNew Strategies")
	}
}

func (sc *ServiceContainer) GetScoped(serviceType reflect.Type, scopeID string) (interface{}, error) {
	sc.mu.RLock()
	descriptor, exists := sc.descriptors[serviceType]
	sc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("service type %v not registered", serviceType)
	}

	switch descriptor.Strategy {
	case ScopedResource:
		return sc.getScopedInstance(descriptor, serviceType, scopeID)
	case SharedResource:
		return sc.getSharedInstance(descriptor)
	case AlwaysNew:
		return descriptor.Factory(sc)
	default:
		return nil, fmt.Errorf("Unknown strategy: %v", descriptor.Strategy)
	}
}

// CreateScope creates a new scope for scoped services
func (sc *ServiceContainer) CreateScope(scopeID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.scopes[scopeID] == nil {
		sc.scopes[scopeID] = make(map[reflect.Type]interface{})
	}
}

// DisposeScope disposes all services in a scope and cleans up resources
func (sc *ServiceContainer) DisposeScope(scopeID string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	scope, exist := sc.scopes[scopeID]
	if !exist {
		return nil
	}

	for _, service := range scope {
		if disposable, ok := service.(interface{ Dispose() error }); ok {
			if err := disposable.Dispose(); err != nil {
				sc.logger.Error().Err(err).Msg("Error disposing service")
			}
		}
	}

	delete(sc.scopes, scopeID)
	return nil
}

// getSharedInstance gets or creates a shared instance
func (sc *ServiceContainer) getSharedInstance(descriptor *ServiceDescriptor) (interface{}, error) {
	if descriptor.SharedInstance != nil {
		return descriptor.SharedInstance, nil
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Double-check pattern
	if descriptor.SharedInstance != nil {
		return descriptor.SharedInstance, nil
	}

	// Create instance
	instance, err := descriptor.Factory(sc)
	if err != nil {
		return nil, err
	}

	descriptor.SharedInstance = instance
	return instance, nil
}

// getScopedInstance gets or creates a scoped instance
func (sc *ServiceContainer) getScopedInstance(descriptor *ServiceDescriptor, serviceType reflect.Type, scopeID string) (interface{}, error) {
	sc.mu.RLock()
	scope, exists := sc.scopes[scopeID]
	if exists {
		if instance, found := scope[serviceType]; found {
			sc.mu.RUnlock()
			return instance, nil
		}
	}

	sc.mu.RUnlock()

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.scopes[scopeID] == nil {
		sc.scopes[scopeID] = make(map[reflect.Type]interface{})
	}

	if instance, found := sc.scopes[scopeID][serviceType]; found {
		return instance, nil
	}

	instance, err := descriptor.Factory(sc)
	if err != nil {
		return nil, err
	}

	sc.scopes[scopeID][serviceType] = instance
	return instance, nil
}
