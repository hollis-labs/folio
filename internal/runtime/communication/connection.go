package communication

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// ConnectionManager handles persistent connections between agents
type ConnectionManager struct {
	connections map[runtime.AgentID]*Connection
	broker      *MessageBroker
	mu          sync.RWMutex
	started     bool
	stopCh      chan struct{}
	eventCh     chan ConnectionEvent
}

// Connection represents a persistent connection to an agent
type Connection struct {
	AgentID        runtime.AgentID
	Status         ConnectionStatus
	LastHeartbeat  time.Time
	MessageQueue   chan runtime.Message
	RetryCount     int
	MaxRetries     int
	RetryInterval  time.Duration
	Endpoint       string
	Metadata       map[string]string
	Created        time.Time
}

// ConnectionStatus represents the status of a connection
type ConnectionStatus string

const (
	ConnectionStatusConnecting ConnectionStatus = "connecting"
	ConnectionStatusConnected  ConnectionStatus = "connected"
	ConnectionStatusRetrying   ConnectionStatus = "retrying"
	ConnectionStatusFailed     ConnectionStatus = "failed"
	ConnectionStatusClosed     ConnectionStatus = "closed"
)

// ConnectionEvent represents connection status changes
type ConnectionEvent struct {
	Type      ConnectionEventType `json:"type"`
	AgentID   runtime.AgentID     `json:"agent_id"`
	Status    ConnectionStatus    `json:"status"`
	Error     string              `json:"error,omitempty"`
	Timestamp time.Time           `json:"timestamp"`
}

// ConnectionEventType defines different types of connection events
type ConnectionEventType string

const (
	ConnectionEventConnected    ConnectionEventType = "connected"
	ConnectionEventDisconnected ConnectionEventType = "disconnected"
	ConnectionEventRetrying     ConnectionEventType = "retrying"
	ConnectionEventFailed       ConnectionEventType = "failed"
	ConnectionEventHeartbeat    ConnectionEventType = "heartbeat"
)

// MessageRouter handles intelligent message routing
type MessageRouter struct {
	routes     map[runtime.MessageType][]RouteRule
	connMgr    *ConnectionManager
	mu         sync.RWMutex
}

// RouteRule defines how messages should be routed
type RouteRule struct {
	Pattern     string                 `json:"pattern"`     // Agent ID pattern or "*"
	Priority    int                   `json:"priority"`    // Higher priority rules are checked first
	LoadBalance LoadBalanceStrategy   `json:"load_balance"` // How to distribute messages
	Filter      map[string]interface{} `json:"filter"`      // Message payload filters
}

// LoadBalanceStrategy defines how to distribute messages among multiple targets
type LoadBalanceStrategy string

const (
	LoadBalanceRoundRobin LoadBalanceStrategy = "round_robin"
	LoadBalanceRandom     LoadBalanceStrategy = "random"
	LoadBalanceLeastLoad  LoadBalanceStrategy = "least_load"
	LoadBalanceFirst      LoadBalanceStrategy = "first"
)

// MessageTracker tracks message delivery and provides reliability features
type MessageTracker struct {
	messages       map[string]*TrackedMessage
	pendingRetries map[string]*TrackedMessage
	mu             sync.RWMutex
	retryTimer     *time.Timer
}

// TrackedMessage represents a message being tracked for delivery
type TrackedMessage struct {
	Message       runtime.Message `json:"message"`
	Attempts      int            `json:"attempts"`
	MaxAttempts   int            `json:"max_attempts"`
	LastAttempt   time.Time      `json:"last_attempt"`
	NextRetry     time.Time      `json:"next_retry"`
	RetryInterval time.Duration  `json:"retry_interval"`
	Status        MessageStatus  `json:"status"`
	Error         string         `json:"error,omitempty"`
}

// MessageStatus represents the delivery status of a message
type MessageStatus string

const (
	MessageStatusPending    MessageStatus = "pending"
	MessageStatusDelivered  MessageStatus = "delivered"
	MessageStatusFailed     MessageStatus = "failed"
	MessageStatusExpired    MessageStatus = "expired"
	MessageStatusRetrying   MessageStatus = "retrying"
)

// NewConnectionManager creates a new connection manager
func NewConnectionManager(broker *MessageBroker) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[runtime.AgentID]*Connection),
		broker:      broker,
		stopCh:      make(chan struct{}),
		eventCh:     make(chan ConnectionEvent, 100),
	}
}

// Start starts the connection manager
func (cm *ConnectionManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.started {
		return fmt.Errorf("connection manager already started")
	}

	cm.started = true
	go cm.manageConnections(ctx)
	go cm.heartbeatChecker(ctx)

	return nil
}

// Stop stops the connection manager
func (cm *ConnectionManager) Stop(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.started {
		return nil
	}

	close(cm.stopCh)
	cm.started = false

	// Close all connections
	for _, conn := range cm.connections {
		conn.Status = ConnectionStatusClosed
		close(conn.MessageQueue)
	}

	return nil
}

// RegisterConnection registers a new agent connection
func (cm *ConnectionManager) RegisterConnection(agentID runtime.AgentID, endpoint string, metadata map[string]string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn := &Connection{
		AgentID:       agentID,
		Status:        ConnectionStatusConnecting,
		LastHeartbeat: time.Now(),
		MessageQueue:  make(chan runtime.Message, 100),
		MaxRetries:    3,
		RetryInterval: 30 * time.Second,
		Endpoint:      endpoint,
		Metadata:      metadata,
		Created:       time.Now(),
	}

	cm.connections[agentID] = conn

	// Start connection handler
	go cm.handleConnection(conn)

	return nil
}

// UnregisterConnection removes an agent connection
func (cm *ConnectionManager) UnregisterConnection(agentID runtime.AgentID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[agentID]
	if !exists {
		return fmt.Errorf("connection not found for agent %s", agentID)
	}

	conn.Status = ConnectionStatusClosed
	close(conn.MessageQueue)
	delete(cm.connections, agentID)

	// Send disconnection event
	cm.sendConnectionEvent(ConnectionEvent{
		Type:      ConnectionEventDisconnected,
		AgentID:   agentID,
		Status:    ConnectionStatusClosed,
		Timestamp: time.Now(),
	})

	return nil
}

// GetConnection returns the connection for an agent
func (cm *ConnectionManager) GetConnection(agentID runtime.AgentID) (*Connection, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, exists := cm.connections[agentID]
	return conn, exists
}

// GetConnections returns all active connections
func (cm *ConnectionManager) GetConnections() map[runtime.AgentID]*Connection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connections := make(map[runtime.AgentID]*Connection)
	for agentID, conn := range cm.connections {
		connections[agentID] = conn
	}

	return connections
}

// SendMessage sends a message through the appropriate connection
func (cm *ConnectionManager) SendMessage(msg runtime.Message) error {
	conn, exists := cm.GetConnection(msg.To)
	if !exists {
		return fmt.Errorf("no connection found for agent %s", msg.To)
	}

	select {
	case conn.MessageQueue <- msg:
		return nil
	default:
		return fmt.Errorf("message queue full for agent %s", msg.To)
	}
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (cm *ConnectionManager) UpdateHeartbeat(agentID runtime.AgentID) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[agentID]; exists {
		conn.LastHeartbeat = time.Now()
		if conn.Status != ConnectionStatusConnected {
			conn.Status = ConnectionStatusConnected
			conn.RetryCount = 0

			// Send connection event
			cm.sendConnectionEvent(ConnectionEvent{
				Type:      ConnectionEventConnected,
				AgentID:   agentID,
				Status:    ConnectionStatusConnected,
				Timestamp: time.Now(),
			})
		}
	}
}

// manageConnections manages connection lifecycle
func (cm *ConnectionManager) manageConnections(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.checkConnectionHealth()
		}
	}
}

// heartbeatChecker checks for agent heartbeats
func (cm *ConnectionManager) heartbeatChecker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.checkHeartbeats()
		}
	}
}

// handleConnection handles messages for a specific connection
func (cm *ConnectionManager) handleConnection(conn *Connection) {
	for msg := range conn.MessageQueue {
		// In a real implementation, this would send the message to the agent
		// For now, we'll just forward it to the broker
		if err := cm.broker.SendMessage(msg); err != nil {
			fmt.Printf("Failed to send message to agent %s: %v\n", conn.AgentID, err)
		}
	}
}

// checkConnectionHealth checks the health of all connections
func (cm *ConnectionManager) checkConnectionHealth() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for agentID, conn := range cm.connections {
		// Check if connection has been inactive for too long
		if now.Sub(conn.LastHeartbeat) > 2*time.Minute {
			if conn.Status == ConnectionStatusConnected {
				conn.Status = ConnectionStatusRetrying
				cm.sendConnectionEvent(ConnectionEvent{
					Type:      ConnectionEventRetrying,
					AgentID:   agentID,
					Status:    ConnectionStatusRetrying,
					Timestamp: now,
				})
			}
		}
	}
}

// checkHeartbeats checks for missed heartbeats
func (cm *ConnectionManager) checkHeartbeats() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	now := time.Now()
	for agentID, conn := range cm.connections {
		if now.Sub(conn.LastHeartbeat) > 5*time.Minute {
			if conn.Status != ConnectionStatusFailed {
				conn.Status = ConnectionStatusFailed
				cm.sendConnectionEvent(ConnectionEvent{
					Type:      ConnectionEventFailed,
					AgentID:   agentID,
					Status:    ConnectionStatusFailed,
					Error:     "heartbeat timeout",
					Timestamp: now,
				})
			}
		}
	}
}

// sendConnectionEvent sends a connection event
func (cm *ConnectionManager) sendConnectionEvent(event ConnectionEvent) {
	select {
	case cm.eventCh <- event:
	default:
		// Event channel full, skip
	}
}

// GetConnectionEvents returns the connection event channel
func (cm *ConnectionManager) GetConnectionEvents() <-chan ConnectionEvent {
	return cm.eventCh
}

// NewMessageRouter creates a new message router
func NewMessageRouter(connMgr *ConnectionManager) *MessageRouter {
	return &MessageRouter{
		routes:  make(map[runtime.MessageType][]RouteRule),
		connMgr: connMgr,
	}
}

// AddRoute adds a routing rule for a message type
func (mr *MessageRouter) AddRoute(msgType runtime.MessageType, rule RouteRule) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.routes[msgType] = append(mr.routes[msgType], rule)

	// Sort routes by priority (higher first)
	routes := mr.routes[msgType]
	for i := 0; i < len(routes)-1; i++ {
		for j := i + 1; j < len(routes); j++ {
			if routes[i].Priority < routes[j].Priority {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}
}

// RouteMessage routes a message according to the configured rules
func (mr *MessageRouter) RouteMessage(msg runtime.Message) error {
	mr.mu.RLock()
	routes, exists := mr.routes[msg.Type]
	mr.mu.RUnlock()

	if !exists {
		// No routing rules, use direct routing
		return mr.connMgr.SendMessage(msg)
	}

	// Find matching route
	for _, route := range routes {
		if mr.matchesRoute(msg, route) {
			return mr.applyRoute(msg, route)
		}
	}

	// No matching route, use direct routing
	return mr.connMgr.SendMessage(msg)
}

// matchesRoute checks if a message matches a route rule
func (mr *MessageRouter) matchesRoute(msg runtime.Message, rule RouteRule) bool {
	// Check pattern match
	if rule.Pattern != "*" && rule.Pattern != string(msg.To) {
		return false
	}

	// Check payload filters
	for key, expectedValue := range rule.Filter {
		if actualValue, exists := msg.Payload[key]; !exists || actualValue != expectedValue {
			return false
		}
	}

	return true
}

// applyRoute applies a routing rule to a message
func (mr *MessageRouter) applyRoute(msg runtime.Message, rule RouteRule) error {
	// For now, just use direct routing
	// In a full implementation, this would handle load balancing strategies
	return mr.connMgr.SendMessage(msg)
}

// NewMessageTracker creates a new message tracker
func NewMessageTracker() *MessageTracker {
	return &MessageTracker{
		messages:       make(map[string]*TrackedMessage),
		pendingRetries: make(map[string]*TrackedMessage),
	}
}

// TrackMessage starts tracking a message for delivery
func (mt *MessageTracker) TrackMessage(msg runtime.Message, maxAttempts int) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	tracked := &TrackedMessage{
		Message:       msg,
		Attempts:      1,
		MaxAttempts:   maxAttempts,
		LastAttempt:   time.Now(),
		RetryInterval: 5 * time.Second,
		Status:        MessageStatusPending,
	}

	mt.messages[msg.ID] = tracked
}

// MarkDelivered marks a message as successfully delivered
func (mt *MessageTracker) MarkDelivered(messageID string) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	if tracked, exists := mt.messages[messageID]; exists {
		tracked.Status = MessageStatusDelivered
		delete(mt.pendingRetries, messageID)
	}
}

// MarkFailed marks a message as failed and schedules retry if needed
func (mt *MessageTracker) MarkFailed(messageID string, err error) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	tracked, exists := mt.messages[messageID]
	if !exists {
		return
	}

	tracked.Error = err.Error()
	
	if tracked.Attempts < tracked.MaxAttempts {
		tracked.Status = MessageStatusRetrying
		tracked.NextRetry = time.Now().Add(tracked.RetryInterval)
		mt.pendingRetries[messageID] = tracked
		
		// Increase retry interval (exponential backoff)
		tracked.RetryInterval *= 2
		if tracked.RetryInterval > 5*time.Minute {
			tracked.RetryInterval = 5 * time.Minute
		}
	} else {
		tracked.Status = MessageStatusFailed
	}
}

// GetPendingRetries returns messages that need to be retried
func (mt *MessageTracker) GetPendingRetries() []*TrackedMessage {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	var pending []*TrackedMessage
	now := time.Now()

	for messageID, tracked := range mt.pendingRetries {
		if now.After(tracked.NextRetry) {
			pending = append(pending, tracked)
			tracked.Attempts++
			tracked.LastAttempt = now
			
			// Remove from pending if max attempts reached
			if tracked.Attempts >= tracked.MaxAttempts {
				tracked.Status = MessageStatusFailed
				delete(mt.pendingRetries, messageID)
			}
		}
	}

	return pending
}

// GetMessageStatus returns the status of a tracked message
func (mt *MessageTracker) GetMessageStatus(messageID string) (MessageStatus, bool) {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	if tracked, exists := mt.messages[messageID]; exists {
		return tracked.Status, true
	}
	
	return "", false
}

// Cleanup removes old tracked messages
func (mt *MessageTracker) Cleanup(maxAge time.Duration) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	now := time.Now()
	for messageID, tracked := range mt.messages {
		if now.Sub(tracked.LastAttempt) > maxAge {
			delete(mt.messages, messageID)
			delete(mt.pendingRetries, messageID)
		}
	}
}