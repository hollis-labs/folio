package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// Monitor handles health checking and monitoring of agents
type Monitor struct {
	healthStatuses map[runtime.AgentID]*runtime.HealthStatus
	checkers       map[runtime.AgentID]HealthChecker
	mu             sync.RWMutex
	started        bool
	stopCh         chan struct{}
	eventCh        chan HealthEvent
}

// HealthChecker interface for checking agent health
type HealthChecker interface {
	CheckHealth(ctx context.Context, agentID runtime.AgentID) (*runtime.HealthStatus, error)
}

// HealthEvent represents a health status change event
type HealthEvent struct {
	Type      HealthEventType     `json:"type"`
	AgentID   runtime.AgentID     `json:"agent_id"`
	Status    *runtime.HealthStatus `json:"status"`
	Timestamp time.Time           `json:"timestamp"`
}

// HealthEventType defines different types of health events
type HealthEventType string

const (
	HealthEventHealthy   HealthEventType = "healthy"
	HealthEventUnhealthy HealthEventType = "unhealthy"
	HealthEventRecovered HealthEventType = "recovered"
	HealthEventTimeout   HealthEventType = "timeout"
)

// NewMonitor creates a new health monitor
func NewMonitor() *Monitor {
	return &Monitor{
		healthStatuses: make(map[runtime.AgentID]*runtime.HealthStatus),
		checkers:       make(map[runtime.AgentID]HealthChecker),
		stopCh:         make(chan struct{}),
		eventCh:        make(chan HealthEvent, 100),
	}
}

// Start begins health monitoring
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("health monitor already started")
	}

	m.started = true
	
	// Start monitoring goroutine
	go m.monitorHealth(ctx)
	
	return nil
}

// Stop stops health monitoring
func (m *Monitor) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	close(m.stopCh)
	m.started = false
	
	return nil
}

// RegisterAgent registers an agent for health monitoring
func (m *Monitor) RegisterAgent(agentID runtime.AgentID, checker HealthChecker) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return fmt.Errorf("health monitor not started")
	}

	m.checkers[agentID] = checker
	
	// Initialize health status
	m.healthStatuses[agentID] = &runtime.HealthStatus{
		AgentID:   agentID,
		Status:    "unknown",
		LastCheck: time.Now(),
		Errors:    []string{},
		Metrics: runtime.Metrics{
			CPUUsage:    0,
			MemoryUsage: 0,
			RequestRate: 0,
			ErrorRate:   0,
		},
	}

	return nil
}

// UnregisterAgent removes an agent from health monitoring
func (m *Monitor) UnregisterAgent(agentID runtime.AgentID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.checkers, agentID)
	delete(m.healthStatuses, agentID)

	return nil
}

// GetHealthStatus returns the current health status of an agent
func (m *Monitor) GetHealthStatus(ctx context.Context, agentID runtime.AgentID) (*runtime.HealthStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, exists := m.healthStatuses[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// Return a copy
	statusCopy := *status
	return &statusCopy, nil
}

// GetAllHealthStatuses returns all current health statuses
func (m *Monitor) GetAllHealthStatuses(ctx context.Context) ([]*runtime.HealthStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]*runtime.HealthStatus, 0, len(m.healthStatuses))
	for _, status := range m.healthStatuses {
		statusCopy := *status
		statuses = append(statuses, &statusCopy)
	}

	return statuses, nil
}

// CheckAgentHealth manually triggers a health check for a specific agent
func (m *Monitor) CheckAgentHealth(ctx context.Context, agentID runtime.AgentID) (*runtime.HealthStatus, error) {
	m.mu.RLock()
	checker, exists := m.checkers[agentID]
	if !exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("agent %s not registered for health monitoring", agentID)
	}
	m.mu.RUnlock()

	status, err := checker.CheckHealth(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	m.updateHealthStatus(agentID, status)
	return status, nil
}

// Events returns the health event channel
func (m *Monitor) Events() <-chan HealthEvent {
	return m.eventCh
}

// monitorHealth runs periodic health checks
func (m *Monitor) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performHealthChecks(ctx)
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performHealthChecks runs health checks for all registered agents
func (m *Monitor) performHealthChecks(ctx context.Context) {
	m.mu.RLock()
	checkers := make(map[runtime.AgentID]HealthChecker)
	for id, checker := range m.checkers {
		checkers[id] = checker
	}
	m.mu.RUnlock()

	for agentID, checker := range checkers {
		go m.checkSingleAgent(ctx, agentID, checker)
	}
}

// checkSingleAgent performs a health check for a single agent
func (m *Monitor) checkSingleAgent(ctx context.Context, agentID runtime.AgentID, checker HealthChecker) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	status, err := checker.CheckHealth(checkCtx, agentID)
	responseTime := time.Since(start)

	if err != nil {
		// Health check failed
		status = &runtime.HealthStatus{
			AgentID:      agentID,
			Status:       "unhealthy",
			LastCheck:    time.Now(),
			Errors:       []string{err.Error()},
			ResponseTime: responseTime,
			Metrics: runtime.Metrics{
				CPUUsage:    0,
				MemoryUsage: 0,
				RequestRate: 0,
				ErrorRate:   1.0,
			},
		}
	} else {
		status.ResponseTime = responseTime
	}

	m.updateHealthStatus(agentID, status)
}

// updateHealthStatus updates the health status and emits events
func (m *Monitor) updateHealthStatus(agentID runtime.AgentID, newStatus *runtime.HealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldStatus, exists := m.healthStatuses[agentID]
	m.healthStatuses[agentID] = newStatus

	// Emit health event if status changed
	if !exists || oldStatus.Status != newStatus.Status {
		var eventType HealthEventType
		switch newStatus.Status {
		case "healthy":
			if exists && oldStatus.Status == "unhealthy" {
				eventType = HealthEventRecovered
			} else {
				eventType = HealthEventHealthy
			}
		case "unhealthy":
			eventType = HealthEventUnhealthy
		default:
			return // Don't emit event for unknown status
		}

		m.emitEvent(HealthEvent{
			Type:      eventType,
			AgentID:   agentID,
			Status:    newStatus,
			Timestamp: time.Now(),
		})
	}
}

// emitEvent sends a health event to the event channel
func (m *Monitor) emitEvent(event HealthEvent) {
	select {
	case m.eventCh <- event:
	default:
		// Channel is full, drop event
	}
}

// SimpleHealthChecker implements a basic health checker
type SimpleHealthChecker struct {
	pingFunc func(ctx context.Context, agentID runtime.AgentID) error
}

// NewSimpleHealthChecker creates a new simple health checker
func NewSimpleHealthChecker(pingFunc func(ctx context.Context, agentID runtime.AgentID) error) *SimpleHealthChecker {
	return &SimpleHealthChecker{
		pingFunc: pingFunc,
	}
}

// CheckHealth implements the HealthChecker interface
func (c *SimpleHealthChecker) CheckHealth(ctx context.Context, agentID runtime.AgentID) (*runtime.HealthStatus, error) {
	start := time.Now()
	err := c.pingFunc(ctx, agentID)
	responseTime := time.Since(start)

	status := &runtime.HealthStatus{
		AgentID:      agentID,
		LastCheck:    time.Now(),
		ResponseTime: responseTime,
		Errors:       []string{},
		Metrics: runtime.Metrics{
			CPUUsage:    0,
			MemoryUsage: 0,
			RequestRate: 0,
			ErrorRate:   0,
		},
	}

	if err != nil {
		status.Status = "unhealthy"
		status.Errors = []string{err.Error()}
		status.Metrics.ErrorRate = 1.0
		return status, nil
	}

	status.Status = "healthy"
	return status, nil
}

// RecoveryManager handles automatic recovery of unhealthy agents
type RecoveryManager struct {
	monitor      *Monitor
	agentManager AgentManager
	policies     map[runtime.AgentID]RecoveryPolicy
	mu           sync.RWMutex
	started      bool
	stopCh       chan struct{}
}

// AgentManager interface for agent lifecycle operations
type AgentManager interface {
	RestartAgent(ctx context.Context, id runtime.AgentID) error
	StopAgent(ctx context.Context, id runtime.AgentID) error
	GetAgent(ctx context.Context, id runtime.AgentID) (*runtime.Agent, error)
}

// RecoveryPolicy defines how to recover an unhealthy agent
type RecoveryPolicy struct {
	MaxRetries    int           `json:"max_retries"`
	RetryInterval time.Duration `json:"retry_interval"`
	Action        RecoveryAction `json:"action"`
}

// RecoveryAction defines what action to take for recovery
type RecoveryAction string

const (
	RecoveryActionRestart RecoveryAction = "restart"
	RecoveryActionStop    RecoveryAction = "stop"
	RecoveryActionNone    RecoveryAction = "none"
)

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(monitor *Monitor, agentManager AgentManager) *RecoveryManager {
	return &RecoveryManager{
		monitor:      monitor,
		agentManager: agentManager,
		policies:     make(map[runtime.AgentID]RecoveryPolicy),
		stopCh:       make(chan struct{}),
	}
}

// Start begins automatic recovery operations
func (rm *RecoveryManager) Start(ctx context.Context) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.started {
		return fmt.Errorf("recovery manager already started")
	}

	rm.started = true
	
	// Start recovery goroutine
	go rm.handleRecovery(ctx)
	
	return nil
}

// Stop stops automatic recovery operations
func (rm *RecoveryManager) Stop(ctx context.Context) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.started {
		return nil
	}

	close(rm.stopCh)
	rm.started = false
	
	return nil
}

// SetRecoveryPolicy sets the recovery policy for an agent
func (rm *RecoveryManager) SetRecoveryPolicy(agentID runtime.AgentID, policy RecoveryPolicy) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.policies[agentID] = policy
}

// handleRecovery processes health events and triggers recovery actions
func (rm *RecoveryManager) handleRecovery(ctx context.Context) {
	events := rm.monitor.Events()
	retryAttempts := make(map[runtime.AgentID]int)

	for {
		select {
		case event := <-events:
			if event.Type == HealthEventUnhealthy {
				rm.processRecovery(ctx, event.AgentID, retryAttempts)
			} else if event.Type == HealthEventRecovered {
				// Reset retry count on recovery
				delete(retryAttempts, event.AgentID)
			}
		case <-rm.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// processRecovery processes recovery for an unhealthy agent
func (rm *RecoveryManager) processRecovery(ctx context.Context, agentID runtime.AgentID, retryAttempts map[runtime.AgentID]int) {
	rm.mu.RLock()
	policy, exists := rm.policies[agentID]
	rm.mu.RUnlock()

	if !exists {
		return // No recovery policy defined
	}

	currentAttempts := retryAttempts[agentID]
	if currentAttempts >= policy.MaxRetries {
		return // Max retries exceeded
	}

	retryAttempts[agentID] = currentAttempts + 1

	// Wait for retry interval
	time.Sleep(policy.RetryInterval)

	switch policy.Action {
	case RecoveryActionRestart:
		rm.agentManager.RestartAgent(ctx, agentID)
	case RecoveryActionStop:
		rm.agentManager.StopAgent(ctx, agentID)
	case RecoveryActionNone:
		// Do nothing
	}
}