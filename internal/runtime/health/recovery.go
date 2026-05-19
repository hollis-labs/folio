package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// RecoveryManager handles automatic recovery of unhealthy agents
type RecoveryManager struct {
	monitor         *Monitor
	agentManager    AgentManager
	policies        map[runtime.AgentID]RecoveryPolicy
	activeRecoveries map[runtime.AgentID]*RecoverySession
	mu              sync.RWMutex
	started         bool
	stopCh          chan struct{}
	eventCh         chan RecoveryEvent
}

// AgentManager interface for agent operations needed by recovery
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
	BackoffFactor float64       `json:"backoff_factor"`
	MaxInterval   time.Duration `json:"max_interval"`
	HealthyWindow time.Duration `json:"healthy_window"` // Time agent must be healthy before considering recovery successful
}

// RecoveryAction defines what action to take for recovery
type RecoveryAction string

const (
	RecoveryActionRestart  RecoveryAction = "restart"
	RecoveryActionStop     RecoveryAction = "stop"
	RecoveryActionNotify   RecoveryAction = "notify"
	RecoveryActionEscalate RecoveryAction = "escalate"
)

// RecoverySession tracks an ongoing recovery attempt
type RecoverySession struct {
	AgentID       runtime.AgentID `json:"agent_id"`
	Policy        RecoveryPolicy  `json:"policy"`
	Attempts      int             `json:"attempts"`
	LastAttempt   time.Time       `json:"last_attempt"`
	NextAttempt   time.Time       `json:"next_attempt"`
	CurrentInterval time.Duration `json:"current_interval"`
	StartedAt     time.Time       `json:"started_at"`
	Status        RecoveryStatus  `json:"status"`
	Error         string          `json:"error,omitempty"`
}

// RecoveryStatus represents the status of a recovery session
type RecoveryStatus string

const (
	RecoveryStatusActive     RecoveryStatus = "active"
	RecoveryStatusSuccessful RecoveryStatus = "successful"
	RecoveryStatusFailed     RecoveryStatus = "failed"
	RecoveryStatusAborted    RecoveryStatus = "aborted"
)

// RecoveryEvent represents recovery-related events
type RecoveryEvent struct {
	Type      RecoveryEventType   `json:"type"`
	AgentID   runtime.AgentID     `json:"agent_id"`
	Session   *RecoverySession    `json:"session,omitempty"`
	Error     string              `json:"error,omitempty"`
	Timestamp time.Time           `json:"timestamp"`
}

// RecoveryEventType defines different types of recovery events
type RecoveryEventType string

const (
	RecoveryEventStarted    RecoveryEventType = "started"
	RecoveryEventAttempt    RecoveryEventType = "attempt"
	RecoveryEventSuccessful RecoveryEventType = "successful"
	RecoveryEventFailed     RecoveryEventType = "failed"
	RecoveryEventAborted    RecoveryEventType = "aborted"
	RecoveryEventEscalated  RecoveryEventType = "escalated"
)

// MetricsCollector collects performance metrics from agents
type MetricsCollector struct {
	collectors map[runtime.AgentID]MetricCollector
	metrics    map[runtime.AgentID]*runtime.Metrics
	mu         sync.RWMutex
}

// MetricCollector interface for collecting metrics from an agent
type MetricCollector interface {
	CollectMetrics(ctx context.Context, agentID runtime.AgentID) (*runtime.Metrics, error)
}

// HealthcheckRegistry manages different types of health checkers
type HealthcheckRegistry struct {
	checkers map[string]HealthCheckerFactory
	mu       sync.RWMutex
}

// HealthCheckerFactory creates health checkers
type HealthCheckerFactory interface {
	CreateChecker(config map[string]interface{}) (HealthChecker, error)
	GetName() string
	GetDescription() string
}

// SimpleHealthChecker is a basic health checker that uses a provided function
type SimpleHealthChecker struct {
	checkFunc func(ctx context.Context, agentID runtime.AgentID) error
}

// HTTPHealthChecker performs HTTP-based health checks
type HTTPHealthChecker struct {
	endpoint string
	timeout  time.Duration
	method   string
	headers  map[string]string
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(monitor *Monitor, agentManager AgentManager) *RecoveryManager {
	return &RecoveryManager{
		monitor:          monitor,
		agentManager:     agentManager,
		policies:         make(map[runtime.AgentID]RecoveryPolicy),
		activeRecoveries: make(map[runtime.AgentID]*RecoverySession),
		stopCh:           make(chan struct{}),
		eventCh:          make(chan RecoveryEvent, 100),
	}
}

// Start starts the recovery manager
func (rm *RecoveryManager) Start(ctx context.Context) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.started {
		return fmt.Errorf("recovery manager already started")
	}

	rm.started = true

	// Subscribe to health events
	healthEvents := rm.monitor.GetHealthEvents()
	go rm.processHealthEvents(ctx, healthEvents)
	go rm.processRecoveryAttempts(ctx)

	return nil
}

// Stop stops the recovery manager
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

// GetRecoveryPolicy returns the recovery policy for an agent
func (rm *RecoveryManager) GetRecoveryPolicy(agentID runtime.AgentID) (RecoveryPolicy, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	policy, exists := rm.policies[agentID]
	return policy, exists
}

// GetActiveRecoveries returns all active recovery sessions
func (rm *RecoveryManager) GetActiveRecoveries() map[runtime.AgentID]*RecoverySession {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	sessions := make(map[runtime.AgentID]*RecoverySession)
	for agentID, session := range rm.activeRecoveries {
		sessions[agentID] = session
	}

	return sessions
}

// AbortRecovery aborts an ongoing recovery session
func (rm *RecoveryManager) AbortRecovery(agentID runtime.AgentID) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	session, exists := rm.activeRecoveries[agentID]
	if !exists {
		return fmt.Errorf("no active recovery session for agent %s", agentID)
	}

	session.Status = RecoveryStatusAborted
	delete(rm.activeRecoveries, agentID)

	rm.sendRecoveryEvent(RecoveryEvent{
		Type:      RecoveryEventAborted,
		AgentID:   agentID,
		Session:   session,
		Timestamp: time.Now(),
	})

	return nil
}

// processHealthEvents processes health events and triggers recovery if needed
func (rm *RecoveryManager) processHealthEvents(ctx context.Context, healthEvents <-chan HealthEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopCh:
			return
		case event := <-healthEvents:
			rm.handleHealthEvent(event)
		}
	}
}

// handleHealthEvent handles a single health event
func (rm *RecoveryManager) handleHealthEvent(event HealthEvent) {
	switch event.Type {
	case HealthEventUnhealthy:
		rm.startRecovery(event.AgentID)
	case HealthEventHealthy:
		rm.completeRecovery(event.AgentID)
	case HealthEventTimeout:
		rm.startRecovery(event.AgentID)
	}
}

// startRecovery starts a recovery session for an agent
func (rm *RecoveryManager) startRecovery(agentID runtime.AgentID) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if recovery is already active
	if _, exists := rm.activeRecoveries[agentID]; exists {
		return
	}

	// Get recovery policy
	policy, exists := rm.policies[agentID]
	if !exists {
		// No policy defined, skip recovery
		return
	}

	// Create recovery session
	session := &RecoverySession{
		AgentID:         agentID,
		Policy:          policy,
		Attempts:        0,
		CurrentInterval: policy.RetryInterval,
		StartedAt:       time.Now(),
		Status:          RecoveryStatusActive,
	}

	rm.activeRecoveries[agentID] = session

	// Send event
	rm.sendRecoveryEvent(RecoveryEvent{
		Type:      RecoveryEventStarted,
		AgentID:   agentID,
		Session:   session,
		Timestamp: time.Now(),
	})

	// Schedule first attempt
	rm.scheduleRecoveryAttempt(session)
}

// completeRecovery marks a recovery session as successful
func (rm *RecoveryManager) completeRecovery(agentID runtime.AgentID) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	session, exists := rm.activeRecoveries[agentID]
	if !exists {
		return
	}

	// Check if agent has been healthy for the required window
	if time.Since(session.LastAttempt) >= session.Policy.HealthyWindow {
		session.Status = RecoveryStatusSuccessful
		delete(rm.activeRecoveries, agentID)

		rm.sendRecoveryEvent(RecoveryEvent{
			Type:      RecoveryEventSuccessful,
			AgentID:   agentID,
			Session:   session,
			Timestamp: time.Now(),
		})
	}
}

// scheduleRecoveryAttempt schedules the next recovery attempt
func (rm *RecoveryManager) scheduleRecoveryAttempt(session *RecoverySession) {
	session.NextAttempt = time.Now().Add(session.CurrentInterval)
}

// processRecoveryAttempts processes scheduled recovery attempts
func (rm *RecoveryManager) processRecoveryAttempts(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopCh:
			return
		case <-ticker.C:
			rm.processScheduledAttempts(ctx)
		}
	}
}

// processScheduledAttempts processes all scheduled recovery attempts
func (rm *RecoveryManager) processScheduledAttempts(ctx context.Context) {
	rm.mu.Lock()
	var attemptList []*RecoverySession
	now := time.Now()

	for _, session := range rm.activeRecoveries {
		if session.Status == RecoveryStatusActive && now.After(session.NextAttempt) {
			attemptList = append(attemptList, session)
		}
	}
	rm.mu.Unlock()

	// Process attempts outside of lock
	for _, session := range attemptList {
		rm.attemptRecovery(ctx, session)
	}
}

// attemptRecovery attempts to recover an agent
func (rm *RecoveryManager) attemptRecovery(ctx context.Context, session *RecoverySession) {
	session.Attempts++
	session.LastAttempt = time.Now()

	// Send attempt event
	rm.sendRecoveryEvent(RecoveryEvent{
		Type:      RecoveryEventAttempt,
		AgentID:   session.AgentID,
		Session:   session,
		Timestamp: time.Now(),
	})

	var err error

	switch session.Policy.Action {
	case RecoveryActionRestart:
		err = rm.agentManager.RestartAgent(ctx, session.AgentID)
	case RecoveryActionStop:
		err = rm.agentManager.StopAgent(ctx, session.AgentID)
	case RecoveryActionNotify:
		// Send notification (implementation would depend on notification system)
		fmt.Printf("RECOVERY NOTIFICATION: Agent %s requires attention\n", session.AgentID)
	case RecoveryActionEscalate:
		// Escalate to higher-level recovery system
		rm.sendRecoveryEvent(RecoveryEvent{
			Type:      RecoveryEventEscalated,
			AgentID:   session.AgentID,
			Session:   session,
			Timestamp: time.Now(),
		})
	}

	if err != nil {
		session.Error = err.Error()

		// Check if we've exceeded max retries
		if session.Attempts >= session.Policy.MaxRetries {
			rm.mu.Lock()
			session.Status = RecoveryStatusFailed
			delete(rm.activeRecoveries, session.AgentID)
			rm.mu.Unlock()

			rm.sendRecoveryEvent(RecoveryEvent{
				Type:      RecoveryEventFailed,
				AgentID:   session.AgentID,
				Session:   session,
				Error:     err.Error(),
				Timestamp: time.Now(),
			})
		} else {
			// Schedule next attempt with backoff
			session.CurrentInterval = time.Duration(float64(session.CurrentInterval) * session.Policy.BackoffFactor)
			if session.CurrentInterval > session.Policy.MaxInterval {
				session.CurrentInterval = session.Policy.MaxInterval
			}
			rm.scheduleRecoveryAttempt(session)
		}
	}
}

// sendRecoveryEvent sends a recovery event
func (rm *RecoveryManager) sendRecoveryEvent(event RecoveryEvent) {
	select {
	case rm.eventCh <- event:
	default:
		// Event channel full, skip
	}
}

// GetRecoveryEvents returns the recovery event channel
func (rm *RecoveryManager) GetRecoveryEvents() <-chan RecoveryEvent {
	return rm.eventCh
}

// NewSimpleHealthChecker creates a simple health checker
func NewSimpleHealthChecker(checkFunc func(ctx context.Context, agentID runtime.AgentID) error) *SimpleHealthChecker {
	return &SimpleHealthChecker{
		checkFunc: checkFunc,
	}
}

// CheckHealth performs a health check
func (shc *SimpleHealthChecker) CheckHealth(ctx context.Context, agentID runtime.AgentID) (*runtime.HealthStatus, error) {
	start := time.Now()
	err := shc.checkFunc(ctx, agentID)
	responseTime := time.Since(start)

	status := &runtime.HealthStatus{
		AgentID:      agentID,
		LastCheck:    time.Now(),
		ResponseTime: responseTime,
	}

	if err != nil {
		status.Status = "unhealthy"
		status.Errors = []string{err.Error()}
	} else {
		status.Status = "healthy"
		status.Errors = nil
	}

	return status, nil
}

// NewHTTPHealthChecker creates an HTTP health checker
func NewHTTPHealthChecker(endpoint string, timeout time.Duration) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		endpoint: endpoint,
		timeout:  timeout,
		method:   "GET",
		headers:  make(map[string]string),
	}
}

// CheckHealth performs an HTTP health check
func (hhc *HTTPHealthChecker) CheckHealth(ctx context.Context, agentID runtime.AgentID) (*runtime.HealthStatus, error) {
	// Implementation would make actual HTTP request
	// For now, return a basic status
	status := &runtime.HealthStatus{
		AgentID:      agentID,
		Status:       "healthy",
		LastCheck:    time.Now(),
		Errors:       nil,
		ResponseTime: 100 * time.Millisecond,
	}

	return status, nil
}

// SetMethod sets the HTTP method for health checks
func (hhc *HTTPHealthChecker) SetMethod(method string) {
	hhc.method = method
}

// SetHeader sets a header for health check requests
func (hhc *HTTPHealthChecker) SetHeader(key, value string) {
	hhc.headers[key] = value
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		collectors: make(map[runtime.AgentID]MetricCollector),
		metrics:    make(map[runtime.AgentID]*runtime.Metrics),
	}
}

// RegisterCollector registers a metric collector for an agent
func (mc *MetricsCollector) RegisterCollector(agentID runtime.AgentID, collector MetricCollector) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.collectors[agentID] = collector
}

// UnregisterCollector removes a metric collector for an agent
func (mc *MetricsCollector) UnregisterCollector(agentID runtime.AgentID) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	delete(mc.collectors, agentID)
	delete(mc.metrics, agentID)
}

// CollectMetrics collects metrics from all registered agents
func (mc *MetricsCollector) CollectMetrics(ctx context.Context) error {
	mc.mu.RLock()
	collectors := make(map[runtime.AgentID]MetricCollector)
	for agentID, collector := range mc.collectors {
		collectors[agentID] = collector
	}
	mc.mu.RUnlock()

	for agentID, collector := range collectors {
		metrics, err := collector.CollectMetrics(ctx, agentID)
		if err != nil {
			fmt.Printf("Failed to collect metrics for agent %s: %v\n", agentID, err)
			continue
		}

		mc.mu.Lock()
		mc.metrics[agentID] = metrics
		mc.mu.Unlock()
	}

	return nil
}

// GetMetrics returns the latest metrics for an agent
func (mc *MetricsCollector) GetMetrics(agentID runtime.AgentID) (*runtime.Metrics, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	metrics, exists := mc.metrics[agentID]
	return metrics, exists
}

// GetAllMetrics returns metrics for all agents
func (mc *MetricsCollector) GetAllMetrics() map[runtime.AgentID]*runtime.Metrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	result := make(map[runtime.AgentID]*runtime.Metrics)
	for agentID, metrics := range mc.metrics {
		result[agentID] = metrics
	}

	return result
}

// NewHealthcheckRegistry creates a new healthcheck registry
func NewHealthcheckRegistry() *HealthcheckRegistry {
	return &HealthcheckRegistry{
		checkers: make(map[string]HealthCheckerFactory),
	}
}

// RegisterChecker registers a health checker factory
func (hr *HealthcheckRegistry) RegisterChecker(factory HealthCheckerFactory) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.checkers[factory.GetName()] = factory
}

// CreateChecker creates a health checker by name
func (hr *HealthcheckRegistry) CreateChecker(name string, config map[string]interface{}) (HealthChecker, error) {
	hr.mu.RLock()
	factory, exists := hr.checkers[name]
	hr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown health checker type: %s", name)
	}

	return factory.CreateChecker(config)
}

// ListCheckers returns all registered health checker names
func (hr *HealthcheckRegistry) ListCheckers() []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	var names []string
	for name := range hr.checkers {
		names = append(names, name)
	}

	return names
}