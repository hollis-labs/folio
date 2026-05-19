package runtime

import (
	"context"
	"time"
)

// AgentState represents the current state of an agent
type AgentState string

const (
	AgentStateStarting   AgentState = "starting"
	AgentStateRunning    AgentState = "running"
	AgentStateStopping   AgentState = "stopping"
	AgentStateStopped    AgentState = "stopped"
	AgentStateError      AgentState = "error"
	AgentStateRecovering AgentState = "recovering"
)

// AgentID is a unique identifier for an agent instance
type AgentID string

// Agent represents a runtime agent instance
type Agent struct {
	ID          AgentID           `json:"id"`
	Name        string            `json:"name"`
	State       AgentState        `json:"state"`
	Config      AgentConfig       `json:"config"`
	Metadata    map[string]string `json:"metadata"`
	StartedAt   time.Time         `json:"started_at"`
	LastSeen    time.Time         `json:"last_seen"`
	HealthScore float64           `json:"health_score"` // 0.0-1.0
}

// AgentConfig contains configuration for an agent
type AgentConfig struct {
	Image           string            `json:"image"`
	Environment     map[string]string `json:"environment"`
	Resources       ResourceLimits    `json:"resources"`
	HealthCheck     HealthCheckConfig `json:"health_check"`
	RestartPolicy   RestartPolicy     `json:"restart_policy"`
	CommunicationID string            `json:"communication_id"`
}

// ResourceLimits defines resource constraints for an agent
type ResourceLimits struct {
	Memory string `json:"memory"` // e.g., "512Mi"
	CPU    string `json:"cpu"`    // e.g., "0.5"
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	Interval    time.Duration `json:"interval"`
	Timeout     time.Duration `json:"timeout"`
	Retries     int           `json:"retries"`
	StartPeriod time.Duration `json:"start_period"`
}

// RestartPolicy defines how agents should be restarted
type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "always"
	RestartPolicyOnFailure RestartPolicy = "on-failure"
	RestartPolicyNever     RestartPolicy = "never"
)

// Message represents inter-agent communication
type Message struct {
	ID        string                 `json:"id"`
	From      AgentID                `json:"from"`
	To        AgentID                `json:"to"`
	Type      MessageType            `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
	TTL       time.Duration          `json:"ttl"`
}

// MessageType defines different types of inter-agent messages
type MessageType string

const (
	MessageTypeCommand      MessageType = "command"
	MessageTypeResponse     MessageType = "response"
	MessageTypeNotification MessageType = "notification"
	MessageTypeHeartbeat    MessageType = "heartbeat"
	MessageTypeDiscovery    MessageType = "discovery"
)

// ServiceInfo represents a discoverable service
type ServiceInfo struct {
	Name      string            `json:"name"`
	AgentID   AgentID           `json:"agent_id"`
	Endpoint  string            `json:"endpoint"`
	Metadata  map[string]string `json:"metadata"`
	TTL       time.Duration     `json:"ttl"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// HealthStatus represents the health status of an agent
type HealthStatus struct {
	AgentID     AgentID   `json:"agent_id"`
	Status      string    `json:"status"` // healthy, unhealthy, unknown
	LastCheck   time.Time `json:"last_check"`
	Errors      []string  `json:"errors"`
	Metrics     Metrics   `json:"metrics"`
	ResponseTime time.Duration `json:"response_time"`
}

// Metrics contains performance metrics for an agent
type Metrics struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	RequestRate float64 `json:"request_rate"`
	ErrorRate   float64 `json:"error_rate"`
}

// Runtime represents the main agent runtime interface
type Runtime interface {
	// Agent lifecycle management
	StartAgent(ctx context.Context, config AgentConfig) (AgentID, error)
	StopAgent(ctx context.Context, id AgentID) error
	RestartAgent(ctx context.Context, id AgentID) error
	GetAgent(ctx context.Context, id AgentID) (*Agent, error)
	ListAgents(ctx context.Context) ([]*Agent, error)
	
	// State management
	SaveAgentState(ctx context.Context, id AgentID, state interface{}) error
	LoadAgentState(ctx context.Context, id AgentID) (interface{}, error)
	
	// Communication
	SendMessage(ctx context.Context, msg Message) error
	Subscribe(ctx context.Context, agentID AgentID, messageTypes []MessageType) (<-chan Message, error)
	
	// Service discovery
	RegisterService(ctx context.Context, service ServiceInfo) error
	UnregisterService(ctx context.Context, name string, agentID AgentID) error
	DiscoverServices(ctx context.Context, name string) ([]ServiceInfo, error)
	
	// Health monitoring
	GetHealthStatus(ctx context.Context, id AgentID) (*HealthStatus, error)
	GetAllHealthStatuses(ctx context.Context) ([]*HealthStatus, error)
	
	// Runtime control
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}