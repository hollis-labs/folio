package communication

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// MessageBroker handles inter-agent communication
type MessageBroker struct {
	subscribers map[runtime.AgentID]map[runtime.MessageType]*Subscription
	messages    chan runtime.Message
	mu          sync.RWMutex
	started     bool
	stopCh      chan struct{}
}

// Subscription represents a message subscription
type Subscription struct {
	AgentID      runtime.AgentID
	MessageTypes []runtime.MessageType
	Channel      chan runtime.Message
	Created      time.Time
}

// NewMessageBroker creates a new message broker
func NewMessageBroker() *MessageBroker {
	return &MessageBroker{
		subscribers: make(map[runtime.AgentID]map[runtime.MessageType]*Subscription),
		messages:    make(chan runtime.Message, 1000),
		stopCh:      make(chan struct{}),
	}
}

// Start begins message processing
func (mb *MessageBroker) Start(ctx context.Context) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if mb.started {
		return fmt.Errorf("message broker already started")
	}

	mb.started = true
	go mb.processMessages(ctx)
	
	return nil
}

// Stop stops message processing
func (mb *MessageBroker) Stop(ctx context.Context) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if !mb.started {
		return nil
	}

	close(mb.stopCh)
	mb.started = false

	// Close all subscription channels
	for _, agentSubs := range mb.subscribers {
		for _, sub := range agentSubs {
			close(sub.Channel)
		}
	}
	mb.subscribers = make(map[runtime.AgentID]map[runtime.MessageType]*Subscription)

	return nil
}

// SendMessage sends a message to the broker for delivery
func (mb *MessageBroker) SendMessage(ctx context.Context, msg runtime.Message) error {
	if !mb.started {
		return fmt.Errorf("message broker not started")
	}

	// Set timestamp if not already set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Generate ID if not set
	if msg.ID == "" {
		msg.ID = generateMessageID()
	}

	select {
	case mb.messages <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("message queue full")
	}
}

// Subscribe creates a subscription for an agent to receive specific message types
func (mb *MessageBroker) Subscribe(ctx context.Context, agentID runtime.AgentID, messageTypes []runtime.MessageType) (<-chan runtime.Message, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if !mb.started {
		return nil, fmt.Errorf("message broker not started")
	}

	// Initialize agent subscriptions if not exists
	if mb.subscribers[agentID] == nil {
		mb.subscribers[agentID] = make(map[runtime.MessageType]*Subscription)
	}

	// Create subscription channel
	subCh := make(chan runtime.Message, 100)

	// Create subscription for each message type
	for _, msgType := range messageTypes {
		subscription := &Subscription{
			AgentID:      agentID,
			MessageTypes: messageTypes,
			Channel:      subCh,
			Created:      time.Now(),
		}
		mb.subscribers[agentID][msgType] = subscription
	}

	return subCh, nil
}

// Unsubscribe removes a subscription
func (mb *MessageBroker) Unsubscribe(ctx context.Context, agentID runtime.AgentID, messageTypes []runtime.MessageType) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	agentSubs, exists := mb.subscribers[agentID]
	if !exists {
		return nil // Already unsubscribed
	}

	// Remove subscriptions for specified message types
	for _, msgType := range messageTypes {
		if sub, exists := agentSubs[msgType]; exists {
			close(sub.Channel)
			delete(agentSubs, msgType)
		}
	}

	// Remove agent entry if no more subscriptions
	if len(agentSubs) == 0 {
		delete(mb.subscribers, agentID)
	}

	return nil
}

// processMessages processes incoming messages and delivers them to subscribers
func (mb *MessageBroker) processMessages(ctx context.Context) {
	for {
		select {
		case msg := <-mb.messages:
			mb.deliverMessage(msg)
		case <-mb.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// deliverMessage delivers a message to appropriate subscribers
func (mb *MessageBroker) deliverMessage(msg runtime.Message) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	// Check if message has expired
	if msg.TTL > 0 && time.Since(msg.Timestamp) > msg.TTL {
		return // Message expired
	}

	// Deliver to specific recipient if targeted
	if msg.To != "" {
		if agentSubs, exists := mb.subscribers[msg.To]; exists {
			if sub, exists := agentSubs[msg.Type]; exists {
				mb.trySendToChannel(sub.Channel, msg)
			}
		}
		return
	}

	// Broadcast to all subscribers of this message type
	for agentID, agentSubs := range mb.subscribers {
		// Skip sending to the sender
		if agentID == msg.From {
			continue
		}

		if sub, exists := agentSubs[msg.Type]; exists {
			mb.trySendToChannel(sub.Channel, msg)
		}
	}
}

// trySendToChannel attempts to send a message to a channel without blocking
func (mb *MessageBroker) trySendToChannel(ch chan runtime.Message, msg runtime.Message) {
	select {
	case ch <- msg:
		// Message delivered
	default:
		// Channel is full, drop message
	}
}

// GetSubscribers returns information about current subscribers
func (mb *MessageBroker) GetSubscribers() map[runtime.AgentID][]runtime.MessageType {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	result := make(map[runtime.AgentID][]runtime.MessageType)
	for agentID, agentSubs := range mb.subscribers {
		var messageTypes []runtime.MessageType
		for msgType := range agentSubs {
			messageTypes = append(messageTypes, msgType)
		}
		result[agentID] = messageTypes
	}

	return result
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

// ConnectionManager handles agent connections and routing
type ConnectionManager struct {
	connections map[runtime.AgentID]*Connection
	broker      *MessageBroker
	mu          sync.RWMutex
}

// Connection represents an agent connection
type Connection struct {
	AgentID    runtime.AgentID
	Endpoint   string
	Status     ConnectionStatus
	LastPing   time.Time
	Metadata   map[string]string
	Created    time.Time
}

// ConnectionStatus represents the status of a connection
type ConnectionStatus string

const (
	ConnectionStatusConnected    ConnectionStatus = "connected"
	ConnectionStatusDisconnected ConnectionStatus = "disconnected"
	ConnectionStatusReconnecting ConnectionStatus = "reconnecting"
)

// NewConnectionManager creates a new connection manager
func NewConnectionManager(broker *MessageBroker) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[runtime.AgentID]*Connection),
		broker:      broker,
	}
}

// RegisterConnection registers a new agent connection
func (cm *ConnectionManager) RegisterConnection(agentID runtime.AgentID, endpoint string, metadata map[string]string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	connection := &Connection{
		AgentID:  agentID,
		Endpoint: endpoint,
		Status:   ConnectionStatusConnected,
		LastPing: time.Now(),
		Metadata: metadata,
		Created:  time.Now(),
	}

	cm.connections[agentID] = connection
	return nil
}

// UnregisterConnection removes an agent connection
func (cm *ConnectionManager) UnregisterConnection(agentID runtime.AgentID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.connections, agentID)
	return nil
}

// GetConnection retrieves a connection by agent ID
func (cm *ConnectionManager) GetConnection(agentID runtime.AgentID) (*Connection, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, exists := cm.connections[agentID]
	if !exists {
		return nil, fmt.Errorf("connection not found for agent %s", agentID)
	}

	// Return a copy
	connCopy := *conn
	return &connCopy, nil
}

// ListConnections returns all active connections
func (cm *ConnectionManager) ListConnections() []*Connection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connections := make([]*Connection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		connCopy := *conn
		connections = append(connections, &connCopy)
	}

	return connections
}

// UpdateLastPing updates the last ping time for a connection
func (cm *ConnectionManager) UpdateLastPing(agentID runtime.AgentID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[agentID]
	if !exists {
		return fmt.Errorf("connection not found for agent %s", agentID)
	}

	conn.LastPing = time.Now()
	return nil
}