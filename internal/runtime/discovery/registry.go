package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// ServiceRegistry manages service discovery and registration
type ServiceRegistry struct {
	services map[string]map[runtime.AgentID]*runtime.ServiceInfo // service name -> agent id -> service info
	mu       sync.RWMutex
	ttlTimer *time.Timer
	stopCh   chan struct{}
	started  bool
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]map[runtime.AgentID]*runtime.ServiceInfo),
		stopCh:   make(chan struct{}),
	}
}

// Start begins the service registry operations
func (sr *ServiceRegistry) Start(ctx context.Context) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if sr.started {
		return fmt.Errorf("service registry already started")
	}

	sr.started = true
	
	// Start TTL cleanup goroutine
	go sr.cleanupExpiredServices(ctx)
	
	return nil
}

// Stop stops the service registry
func (sr *ServiceRegistry) Stop(ctx context.Context) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if !sr.started {
		return nil
	}

	close(sr.stopCh)
	sr.started = false
	
	return nil
}

// RegisterService registers a new service
func (sr *ServiceRegistry) RegisterService(ctx context.Context, service runtime.ServiceInfo) error {
	if !sr.started {
		return fmt.Errorf("service registry not started")
	}

	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Initialize service map if not exists
	if sr.services[service.Name] == nil {
		sr.services[service.Name] = make(map[runtime.AgentID]*runtime.ServiceInfo)
	}

	// Set update timestamp
	service.UpdatedAt = time.Now()

	// Store the service
	sr.services[service.Name][service.AgentID] = &service

	return nil
}

// UnregisterService removes a service registration
func (sr *ServiceRegistry) UnregisterService(ctx context.Context, name string, agentID runtime.AgentID) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	serviceMap, exists := sr.services[name]
	if !exists {
		return nil // Service not registered
	}

	delete(serviceMap, agentID)

	// Clean up empty service map
	if len(serviceMap) == 0 {
		delete(sr.services, name)
	}

	return nil
}

// DiscoverServices finds all instances of a service
func (sr *ServiceRegistry) DiscoverServices(ctx context.Context, name string) ([]runtime.ServiceInfo, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	serviceMap, exists := sr.services[name]
	if !exists {
		return []runtime.ServiceInfo{}, nil
	}

	var services []runtime.ServiceInfo
	now := time.Now()

	for _, service := range serviceMap {
		// Check if service has expired
		if service.TTL > 0 && now.Sub(service.UpdatedAt) > service.TTL {
			continue // Skip expired service
		}
		services = append(services, *service)
	}

	return services, nil
}

// ListAllServices returns all registered services
func (sr *ServiceRegistry) ListAllServices(ctx context.Context) (map[string][]runtime.ServiceInfo, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	result := make(map[string][]runtime.ServiceInfo)
	now := time.Now()

	for serviceName, serviceMap := range sr.services {
		var services []runtime.ServiceInfo
		
		for _, service := range serviceMap {
			// Check if service has expired
			if service.TTL > 0 && now.Sub(service.UpdatedAt) > service.TTL {
				continue // Skip expired service
			}
			services = append(services, *service)
		}

		if len(services) > 0 {
			result[serviceName] = services
		}
	}

	return result, nil
}

// GetServicesByAgent returns all services registered by a specific agent
func (sr *ServiceRegistry) GetServicesByAgent(ctx context.Context, agentID runtime.AgentID) ([]runtime.ServiceInfo, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	var services []runtime.ServiceInfo
	now := time.Now()

	for _, serviceMap := range sr.services {
		for _, service := range serviceMap {
			if service.AgentID == agentID {
				// Check if service has expired
				if service.TTL > 0 && now.Sub(service.UpdatedAt) > service.TTL {
					continue // Skip expired service
				}
				services = append(services, *service)
			}
		}
	}

	return services, nil
}

// UpdateServiceHealth updates the health status of a service
func (sr *ServiceRegistry) UpdateServiceHealth(ctx context.Context, name string, agentID runtime.AgentID, healthy bool) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	serviceMap, exists := sr.services[name]
	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	service, exists := serviceMap[agentID]
	if !exists {
		return fmt.Errorf("service %s for agent %s not found", name, agentID)
	}

	// Update metadata with health status
	if service.Metadata == nil {
		service.Metadata = make(map[string]string)
	}
	
	if healthy {
		service.Metadata["health"] = "healthy"
	} else {
		service.Metadata["health"] = "unhealthy"
	}
	
	service.UpdatedAt = time.Now()

	return nil
}

// cleanupExpiredServices periodically removes expired services
func (sr *ServiceRegistry) cleanupExpiredServices(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Cleanup every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sr.performCleanup()
		case <-sr.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performCleanup removes expired services
func (sr *ServiceRegistry) performCleanup() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	now := time.Now()
	
	for serviceName, serviceMap := range sr.services {
		for agentID, service := range serviceMap {
			// Check if service has expired
			if service.TTL > 0 && now.Sub(service.UpdatedAt) > service.TTL {
				delete(serviceMap, agentID)
			}
		}

		// Clean up empty service map
		if len(serviceMap) == 0 {
			delete(sr.services, serviceName)
		}
	}
}

// ServiceWatcher watches for service changes
type ServiceWatcher struct {
	registry   *ServiceRegistry
	watchers   map[string][]chan ServiceEvent
	mu         sync.RWMutex
	started    bool
	stopCh     chan struct{}
}

// ServiceEvent represents a service change event
type ServiceEvent struct {
	Type        ServiceEventType        `json:"type"`
	ServiceName string                  `json:"service_name"`
	Service     runtime.ServiceInfo     `json:"service"`
	Timestamp   time.Time               `json:"timestamp"`
}

// ServiceEventType defines different types of service events
type ServiceEventType string

const (
	ServiceEventRegistered   ServiceEventType = "registered"
	ServiceEventUnregistered ServiceEventType = "unregistered"
	ServiceEventHealthChanged ServiceEventType = "health_changed"
	ServiceEventExpired      ServiceEventType = "expired"
)

// NewServiceWatcher creates a new service watcher
func NewServiceWatcher(registry *ServiceRegistry) *ServiceWatcher {
	return &ServiceWatcher{
		registry: registry,
		watchers: make(map[string][]chan ServiceEvent),
		stopCh:   make(chan struct{}),
	}
}

// Start begins watching for service changes
func (sw *ServiceWatcher) Start(ctx context.Context) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.started {
		return fmt.Errorf("service watcher already started")
	}

	sw.started = true
	return nil
}

// Stop stops the service watcher
func (sw *ServiceWatcher) Stop(ctx context.Context) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.started {
		return nil
	}

	close(sw.stopCh)
	sw.started = false

	// Close all watcher channels
	for _, watchers := range sw.watchers {
		for _, ch := range watchers {
			close(ch)
		}
	}
	sw.watchers = make(map[string][]chan ServiceEvent)

	return nil
}

// WatchService watches for changes to a specific service
func (sw *ServiceWatcher) WatchService(ctx context.Context, serviceName string) (<-chan ServiceEvent, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.started {
		return nil, fmt.Errorf("service watcher not started")
	}

	eventCh := make(chan ServiceEvent, 10)
	
	if sw.watchers[serviceName] == nil {
		sw.watchers[serviceName] = make([]chan ServiceEvent, 0)
	}
	
	sw.watchers[serviceName] = append(sw.watchers[serviceName], eventCh)

	return eventCh, nil
}

// NotifyEvent sends an event to all watchers of a service
func (sw *ServiceWatcher) NotifyEvent(event ServiceEvent) {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	if watchers, exists := sw.watchers[event.ServiceName]; exists {
		for _, ch := range watchers {
			select {
			case ch <- event:
			default:
				// Channel is full, drop event
			}
		}
	}
}