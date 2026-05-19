package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// Manager handles agent lifecycle operations
type Manager struct {
	agents    map[runtime.AgentID]*runtime.Agent
	mu        sync.RWMutex
	stateStore StateStore
	executor  Executor
	eventCh   chan Event
}

// StateStore interface for persisting agent state
type StateStore interface {
	Save(ctx context.Context, id runtime.AgentID, data interface{}) error
	Load(ctx context.Context, id runtime.AgentID) (interface{}, error)
	Delete(ctx context.Context, id runtime.AgentID) error
	List(ctx context.Context) ([]runtime.AgentID, error)
}

// Executor interface for actually running agents
type Executor interface {
	Start(ctx context.Context, agent *runtime.Agent) error
	Stop(ctx context.Context, id runtime.AgentID) error
	Restart(ctx context.Context, id runtime.AgentID) error
	GetStatus(ctx context.Context, id runtime.AgentID) (runtime.AgentState, error)
}

// Event represents lifecycle events
type Event struct {
	Type      EventType           `json:"type"`
	AgentID   runtime.AgentID     `json:"agent_id"`
	Timestamp time.Time           `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// EventType defines different lifecycle event types
type EventType string

const (
	EventTypeAgentStarted   EventType = "agent_started"
	EventTypeAgentStopped   EventType = "agent_stopped"
	EventTypeAgentFailed    EventType = "agent_failed"
	EventTypeAgentRecovered EventType = "agent_recovered"
	EventTypeStateChanged   EventType = "state_changed"
)

// NewManager creates a new agent manager
func NewManager(stateStore StateStore, executor Executor) *Manager {
	return &Manager{
		agents:     make(map[runtime.AgentID]*runtime.Agent),
		stateStore: stateStore,
		executor:   executor,
		eventCh:    make(chan Event, 100),
	}
}

// Start initializes the agent manager
func (m *Manager) Start(ctx context.Context) error {
	// Load existing agents from state store
	agentIDs, err := m.stateStore.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to load existing agents: %w", err)
	}

	for _, id := range agentIDs {
		data, err := m.stateStore.Load(ctx, id)
		if err != nil {
			continue // Skip failed loads
		}
		
		if agent, ok := data.(*runtime.Agent); ok {
			m.mu.Lock()
			m.agents[id] = agent
			m.mu.Unlock()
		}
	}

	return nil
}

// StartAgent creates and starts a new agent
func (m *Manager) StartAgent(ctx context.Context, config runtime.AgentConfig) (runtime.AgentID, error) {
	id := runtime.AgentID(generateID())
	
	agent := &runtime.Agent{
		ID:          id,
		Name:        config.CommunicationID,
		State:       runtime.AgentStateStarting,
		Config:      config,
		Metadata:    make(map[string]string),
		StartedAt:   time.Now(),
		LastSeen:    time.Now(),
		HealthScore: 1.0,
	}

	m.mu.Lock()
	m.agents[id] = agent
	m.mu.Unlock()

	// Save to persistent store
	if err := m.stateStore.Save(ctx, id, agent); err != nil {
		m.mu.Lock()
		delete(m.agents, id)
		m.mu.Unlock()
		return "", fmt.Errorf("failed to save agent state: %w", err)
	}

	// Start the agent via executor
	if err := m.executor.Start(ctx, agent); err != nil {
		m.mu.Lock()
		delete(m.agents, id)
		m.mu.Unlock()
		m.stateStore.Delete(ctx, id)
		return "", fmt.Errorf("failed to start agent: %w", err)
	}

	// Update state to running
	agent.State = runtime.AgentStateRunning
	m.stateStore.Save(ctx, id, agent)

	// Emit event
	m.emitEvent(Event{
		Type:      EventTypeAgentStarted,
		AgentID:   id,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"name": agent.Name,
		},
	})

	return id, nil
}

// StopAgent stops an agent
func (m *Manager) StopAgent(ctx context.Context, id runtime.AgentID) error {
	m.mu.Lock()
	agent, exists := m.agents[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s not found", id)
	}

	agent.State = runtime.AgentStateStopping
	m.mu.Unlock()

	// Stop via executor
	if err := m.executor.Stop(ctx, id); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// Update state
	m.mu.Lock()
	agent.State = runtime.AgentStateStopped
	m.mu.Unlock()

	// Save state
	m.stateStore.Save(ctx, id, agent)

	// Emit event
	m.emitEvent(Event{
		Type:      EventTypeAgentStopped,
		AgentID:   id,
		Timestamp: time.Now(),
	})

	return nil
}

// RestartAgent restarts an agent
func (m *Manager) RestartAgent(ctx context.Context, id runtime.AgentID) error {
	m.mu.RLock()
	agent, exists := m.agents[id]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("agent %s not found", id)
	}
	m.mu.RUnlock()

	// Stop first
	if err := m.StopAgent(ctx, id); err != nil {
		return fmt.Errorf("failed to stop agent for restart: %w", err)
	}

	// Wait a moment
	time.Sleep(1 * time.Second)

	// Start again with same config
	_, err := m.StartAgent(ctx, agent.Config)
	if err != nil {
		return fmt.Errorf("failed to start agent after restart: %w", err)
	}

	return nil
}

// GetAgent retrieves an agent by ID
func (m *Manager) GetAgent(ctx context.Context, id runtime.AgentID) (*runtime.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.agents[id]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", id)
	}

	// Create a copy to avoid race conditions
	agentCopy := *agent
	return &agentCopy, nil
}

// ListAgents returns all agents
func (m *Manager) ListAgents(ctx context.Context) ([]*runtime.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*runtime.Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		agentCopy := *agent
		agents = append(agents, &agentCopy)
	}

	return agents, nil
}

// UpdateAgentState updates an agent's state
func (m *Manager) UpdateAgentState(ctx context.Context, id runtime.AgentID, state runtime.AgentState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[id]
	if !exists {
		return fmt.Errorf("agent %s not found", id)
	}

	oldState := agent.State
	agent.State = state
	agent.LastSeen = time.Now()

	// Save to persistent store
	if err := m.stateStore.Save(ctx, id, agent); err != nil {
		return fmt.Errorf("failed to save agent state: %w", err)
	}

	// Emit event if state changed
	if oldState != state {
		m.emitEvent(Event{
			Type:      EventTypeStateChanged,
			AgentID:   id,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"old_state": string(oldState),
				"new_state": string(state),
			},
		})
	}

	return nil
}

// Events returns the event channel
func (m *Manager) Events() <-chan Event {
	return m.eventCh
}

// emitEvent sends an event to the channel
func (m *Manager) emitEvent(event Event) {
	select {
	case m.eventCh <- event:
	default:
		// Channel is full, drop event
	}
}

// generateID generates a unique agent ID
func generateID() string {
	return fmt.Sprintf("agent-%d", time.Now().UnixNano())
}