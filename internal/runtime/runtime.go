package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/hollis-labs/folio/internal/runtime/agent"
	"github.com/hollis-labs/folio/internal/runtime/communication"
	"github.com/hollis-labs/folio/internal/runtime/discovery"
	"github.com/hollis-labs/folio/internal/runtime/health"
	"github.com/hollis-labs/folio/internal/runtime/state"
)

// DefaultRuntime implements the Runtime interface
type DefaultRuntime struct {
	agentManager     *agent.Manager
	messageBroker    *communication.MessageBroker
	connectionMgr    *communication.ConnectionManager
	serviceRegistry  *discovery.ServiceRegistry
	serviceWatcher   *discovery.ServiceWatcher
	healthMonitor    *health.Monitor
	recoveryManager  *health.RecoveryManager

	config  Config
	started bool
	mu      sync.RWMutex
}

// Config contains runtime configuration
type Config struct {
	StateStorePath string `json:"state_store_path"`
	EnableRecovery bool   `json:"enable_recovery"`
}

// DefaultConfig returns default runtime configuration
func DefaultConfig() Config {
	return Config{
		StateStorePath: "./state",
		EnableRecovery: true,
	}
}

// NewRuntime creates a new runtime instance
func NewRuntime(config Config) (*DefaultRuntime, error) {
	// Create state store
	stateStore, err := state.NewFileStateStore(config.StateStorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	// Create executor (using docker by default)
	executor := &DockerExecutor{}

	// Create components
	agentManager := agent.NewManager(stateStore, executor)
	messageBroker := communication.NewMessageBroker()
	connectionMgr := communication.NewConnectionManager(messageBroker)
	serviceRegistry := discovery.NewServiceRegistry()
	serviceWatcher := discovery.NewServiceWatcher(serviceRegistry)
	healthMonitor := health.NewMonitor()

	var recoveryManager *health.RecoveryManager
	if config.EnableRecovery {
		recoveryManager = health.NewRecoveryManager(healthMonitor, agentManager)
	}

	return &DefaultRuntime{
		agentManager:     agentManager,
		messageBroker:    messageBroker,
		connectionMgr:    connectionMgr,
		serviceRegistry:  serviceRegistry,
		serviceWatcher:   serviceWatcher,
		healthMonitor:    healthMonitor,
		recoveryManager:  recoveryManager,
		config:           config,
	}, nil
}

// Start initializes and starts the runtime
func (r *DefaultRuntime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return fmt.Errorf("runtime already started")
	}

	// Start all components
	if err := r.agentManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent manager: %w", err)
	}

	if err := r.messageBroker.Start(ctx); err != nil {
		return fmt.Errorf("failed to start message broker: %w", err)
	}

	if err := r.serviceRegistry.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service registry: %w", err)
	}

	if err := r.serviceWatcher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service watcher: %w", err)
	}

	if err := r.healthMonitor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start health monitor: %w", err)
	}

	if r.recoveryManager != nil {
		if err := r.recoveryManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start recovery manager: %w", err)
		}
	}

	r.started = true
	return nil
}

// Stop shuts down the runtime
func (r *DefaultRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.started {
		return nil
	}

	// Stop components in reverse order
	if r.recoveryManager != nil {
		r.recoveryManager.Stop(ctx)
	}

	r.healthMonitor.Stop(ctx)
	r.serviceWatcher.Stop(ctx)
	r.serviceRegistry.Stop(ctx)
	r.messageBroker.Stop(ctx)

	r.started = false
	return nil
}

// Agent lifecycle management

// StartAgent creates and starts a new agent
func (r *DefaultRuntime) StartAgent(ctx context.Context, config AgentConfig) (AgentID, error) {
	if !r.started {
		return "", fmt.Errorf("runtime not started")
	}

	agentID, err := r.agentManager.StartAgent(ctx, config)
	if err != nil {
		return "", err
	}

	// Register for health monitoring
	checker := health.NewSimpleHealthChecker(func(ctx context.Context, id AgentID) error {
		// Simple ping implementation - could be enhanced based on agent type
		_, err := r.agentManager.GetAgent(ctx, id)
		return err
	})

	if err := r.healthMonitor.RegisterAgent(agentID, checker); err != nil {
		// Log warning but don't fail agent start
		fmt.Printf("Warning: failed to register agent for health monitoring: %v\n", err)
	}

	// Set default recovery policy if recovery is enabled
	if r.recoveryManager != nil {
		policy := health.RecoveryPolicy{
			MaxRetries:    3,
			RetryInterval: 30000000000, // 30 seconds
			Action:        health.RecoveryActionRestart,
		}
		r.recoveryManager.SetRecoveryPolicy(agentID, policy)
	}

	return agentID, nil
}

// StopAgent stops an agent
func (r *DefaultRuntime) StopAgent(ctx context.Context, id AgentID) error {
	if !r.started {
		return fmt.Errorf("runtime not started")
	}

	// Unregister from health monitoring
	r.healthMonitor.UnregisterAgent(id)

	return r.agentManager.StopAgent(ctx, id)
}

// RestartAgent restarts an agent
func (r *DefaultRuntime) RestartAgent(ctx context.Context, id AgentID) error {
	if !r.started {
		return fmt.Errorf("runtime not started")
	}

	return r.agentManager.RestartAgent(ctx, id)
}

// GetAgent retrieves an agent by ID
func (r *DefaultRuntime) GetAgent(ctx context.Context, id AgentID) (*Agent, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	return r.agentManager.GetAgent(ctx, id)
}

// ListAgents returns all agents
func (r *DefaultRuntime) ListAgents(ctx context.Context) ([]*Agent, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	return r.agentManager.ListAgents(ctx)
}

// State management

// SaveAgentState persists agent state
func (r *DefaultRuntime) SaveAgentState(ctx context.Context, id AgentID, state interface{}) error {
	if !r.started {
		return fmt.Errorf("runtime not started")
	}

	// This could be enhanced to save arbitrary state data
	// For now, agent state is managed by the agent manager
	return fmt.Errorf("arbitrary state saving not yet implemented")
}

// LoadAgentState loads agent state
func (r *DefaultRuntime) LoadAgentState(ctx context.Context, id AgentID) (interface{}, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	// This could be enhanced to load arbitrary state data
	// For now, agent state is managed by the agent manager
	return r.agentManager.GetAgent(ctx, id)
}

// Communication

// SendMessage sends a message through the message broker
func (r *DefaultRuntime) SendMessage(ctx context.Context, msg Message) error {
	if !r.started {
		return fmt.Errorf("runtime not started")
	}

	return r.messageBroker.SendMessage(ctx, msg)
}

// Subscribe creates a message subscription
func (r *DefaultRuntime) Subscribe(ctx context.Context, agentID AgentID, messageTypes []MessageType) (<-chan Message, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	return r.messageBroker.Subscribe(ctx, agentID, messageTypes)
}

// Service discovery

// RegisterService registers a service
func (r *DefaultRuntime) RegisterService(ctx context.Context, service ServiceInfo) error {
	if !r.started {
		return fmt.Errorf("runtime not started")
	}

	return r.serviceRegistry.RegisterService(ctx, service)
}

// UnregisterService unregisters a service
func (r *DefaultRuntime) UnregisterService(ctx context.Context, name string, agentID AgentID) error {
	if !r.started {
		return fmt.Errorf("runtime not started")
	}

	return r.serviceRegistry.UnregisterService(ctx, name, agentID)
}

// DiscoverServices finds services by name
func (r *DefaultRuntime) DiscoverServices(ctx context.Context, name string) ([]ServiceInfo, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	return r.serviceRegistry.DiscoverServices(ctx, name)
}

// Health monitoring

// GetHealthStatus returns health status for an agent
func (r *DefaultRuntime) GetHealthStatus(ctx context.Context, id AgentID) (*HealthStatus, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	return r.healthMonitor.GetHealthStatus(ctx, id)
}

// GetAllHealthStatuses returns all health statuses
func (r *DefaultRuntime) GetAllHealthStatuses(ctx context.Context) ([]*HealthStatus, error) {
	if !r.started {
		return nil, fmt.Errorf("runtime not started")
	}

	return r.healthMonitor.GetAllHealthStatuses(ctx)
}