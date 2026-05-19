package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
	"gopkg.in/yaml.v3"
)

// Manager handles runtime configuration management
type Manager struct {
	configStore ConfigStore
	watchers    map[string]chan ConfigEvent
	mu          sync.RWMutex
	started     bool
	stopCh      chan struct{}
}

// ConfigStore interface for persisting configuration data
type ConfigStore interface {
	Save(ctx context.Context, key string, config interface{}) error
	Load(ctx context.Context, key string, config interface{}) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]string, error)
}

// ConfigEvent represents a configuration change event
type ConfigEvent struct {
	Type      ConfigEventType `json:"type"`
	Key       string          `json:"key"`
	Timestamp time.Time       `json:"timestamp"`
	Data      interface{}     `json:"data"`
}

// ConfigEventType defines different types of configuration events
type ConfigEventType string

const (
	ConfigEventCreated ConfigEventType = "created"
	ConfigEventUpdated ConfigEventType = "updated"
	ConfigEventDeleted ConfigEventType = "deleted"
	ConfigEventLoaded  ConfigEventType = "loaded"
)

// RuntimeConfig represents the main runtime configuration
type RuntimeConfig struct {
	// Core runtime settings
	StateStorePath string `yaml:"state_store_path" json:"state_store_path"`
	EnableRecovery bool   `yaml:"enable_recovery" json:"enable_recovery"`
	LogLevel       string `yaml:"log_level" json:"log_level"`
	
	// Agent defaults
	DefaultAgentConfig AgentDefaults `yaml:"agent_defaults" json:"agent_defaults"`
	
	// Health monitoring
	HealthCheck HealthCheckConfig `yaml:"health_check" json:"health_check"`
	
	// Communication
	MessageBroker MessageBrokerConfig `yaml:"message_broker" json:"message_broker"`
	
	// Service discovery
	ServiceDiscovery ServiceDiscoveryConfig `yaml:"service_discovery" json:"service_discovery"`
	
	// Recovery settings
	Recovery RecoveryConfig `yaml:"recovery" json:"recovery"`
}

// AgentDefaults provides default settings for new agents
type AgentDefaults struct {
	Resources     runtime.ResourceLimits     `yaml:"resources" json:"resources"`
	HealthCheck   runtime.HealthCheckConfig  `yaml:"health_check" json:"health_check"`
	RestartPolicy runtime.RestartPolicy      `yaml:"restart_policy" json:"restart_policy"`
	Environment   map[string]string          `yaml:"environment" json:"environment"`
}

// HealthCheckConfig contains health monitoring configuration
type HealthCheckConfig struct {
	CheckInterval    time.Duration `yaml:"check_interval" json:"check_interval"`
	DefaultTimeout   time.Duration `yaml:"default_timeout" json:"default_timeout"`
	MaxRetries       int           `yaml:"max_retries" json:"max_retries"`
	UnhealthyThreshold int         `yaml:"unhealthy_threshold" json:"unhealthy_threshold"`
}

// MessageBrokerConfig contains message broker configuration
type MessageBrokerConfig struct {
	QueueSize     int           `yaml:"queue_size" json:"queue_size"`
	MessageTTL    time.Duration `yaml:"message_ttl" json:"message_ttl"`
	RetryCount    int           `yaml:"retry_count" json:"retry_count"`
	RetryInterval time.Duration `yaml:"retry_interval" json:"retry_interval"`
}

// ServiceDiscoveryConfig contains service discovery configuration
type ServiceDiscoveryConfig struct {
	TTL            time.Duration `yaml:"ttl" json:"ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
	EnableAutoCleanup bool        `yaml:"enable_auto_cleanup" json:"enable_auto_cleanup"`
}

// RecoveryConfig contains recovery mechanism configuration
type RecoveryConfig struct {
	MaxRetries        int           `yaml:"max_retries" json:"max_retries"`
	RetryInterval     time.Duration `yaml:"retry_interval" json:"retry_interval"`
	BackoffMultiplier float64       `yaml:"backoff_multiplier" json:"backoff_multiplier"`
	MaxRetryInterval  time.Duration `yaml:"max_retry_interval" json:"max_retry_interval"`
}

// FileConfigStore implements ConfigStore using the filesystem
type FileConfigStore struct {
	basePath string
	mu       sync.RWMutex
}

// NewFileConfigStore creates a new file-based configuration store
func NewFileConfigStore(basePath string) (*FileConfigStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	return &FileConfigStore{
		basePath: basePath,
	}, nil
}

// Save persists configuration to disk
func (s *FileConfigStore) Save(ctx context.Context, key string, config interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.getFilePath(key)
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Determine format based on file extension
	var data []byte
	var err error
	
	ext := filepath.Ext(filePath)
	switch ext {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(config)
	case ".json":
		data, err = json.MarshalIndent(config, "", "  ")
	default:
		// Default to YAML
		data, err = yaml.Marshal(config)
		filePath = filePath + ".yaml"
	}
	
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// Load retrieves configuration from disk
func (s *FileConfigStore) Load(ctx context.Context, key string, config interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.getFilePath(key)
	
	// Try with different extensions
	extensions := []string{"", ".yaml", ".yml", ".json"}
	var data []byte
	var err error
	var actualPath string
	
	for _, ext := range extensions {
		testPath := filePath + ext
		data, err = os.ReadFile(testPath)
		if err == nil {
			actualPath = testPath
			break
		}
	}
	
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	// Determine format and unmarshal
	ext := filepath.Ext(actualPath)
	switch ext {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, config)
	case ".json":
		err = json.Unmarshal(data, config)
	default:
		// Try YAML first, then JSON
		if yamlErr := yaml.Unmarshal(data, config); yamlErr != nil {
			if jsonErr := json.Unmarshal(data, config); jsonErr != nil {
				return fmt.Errorf("failed to unmarshal as YAML: %v, or JSON: %v", yamlErr, jsonErr)
			}
		}
	}
	
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// Delete removes configuration from disk
func (s *FileConfigStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.getFilePath(key)
	
	// Try to delete with different extensions
	extensions := []string{"", ".yaml", ".yml", ".json"}
	var lastErr error
	
	for _, ext := range extensions {
		testPath := filePath + ext
		if err := os.Remove(testPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			lastErr = err
		}
	}
	
	if lastErr != nil {
		return fmt.Errorf("failed to delete config file: %w", lastErr)
	}
	
	return fmt.Errorf("config file not found: %s", filePath)
}

// List returns all configuration keys
func (s *FileConfigStore) List(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory: %w", err)
	}

	var keys []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		name := entry.Name()
		// Remove common config file extensions
		for _, ext := range []string{".yaml", ".yml", ".json"} {
			if strings.HasSuffix(name, ext) {
				name = strings.TrimSuffix(name, ext)
				break
			}
		}
		
		keys = append(keys, name)
	}

	return keys, nil
}

// getFilePath returns the file path for a configuration key
func (s *FileConfigStore) getFilePath(key string) string {
	return filepath.Join(s.basePath, key)
}

// NewManager creates a new configuration manager
func NewManager(configStore ConfigStore) *Manager {
	return &Manager{
		configStore: configStore,
		watchers:    make(map[string]chan ConfigEvent),
		stopCh:      make(chan struct{}),
	}
}

// Start starts the configuration manager
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("config manager already started")
	}

	m.started = true
	return nil
}

// Stop stops the configuration manager
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	close(m.stopCh)
	m.started = false

	// Close all watcher channels
	for _, ch := range m.watchers {
		close(ch)
	}
	m.watchers = make(map[string]chan ConfigEvent)

	return nil
}

// SaveConfig saves a configuration with the specified key
func (m *Manager) SaveConfig(ctx context.Context, key string, config interface{}) error {
	if err := m.configStore.Save(ctx, key, config); err != nil {
		return err
	}

	// Notify watchers
	m.notifyWatchers(ConfigEvent{
		Type:      ConfigEventCreated,
		Key:       key,
		Timestamp: time.Now(),
		Data:      config,
	})

	return nil
}

// LoadConfig loads a configuration with the specified key
func (m *Manager) LoadConfig(ctx context.Context, key string, config interface{}) error {
	if err := m.configStore.Load(ctx, key, config); err != nil {
		return err
	}

	// Notify watchers
	m.notifyWatchers(ConfigEvent{
		Type:      ConfigEventLoaded,
		Key:       key,
		Timestamp: time.Now(),
		Data:      config,
	})

	return nil
}

// DeleteConfig deletes a configuration with the specified key
func (m *Manager) DeleteConfig(ctx context.Context, key string) error {
	if err := m.configStore.Delete(ctx, key); err != nil {
		return err
	}

	// Notify watchers
	m.notifyWatchers(ConfigEvent{
		Type:      ConfigEventDeleted,
		Key:       key,
		Timestamp: time.Now(),
	})

	return nil
}

// ListConfigs returns all configuration keys
func (m *Manager) ListConfigs(ctx context.Context) ([]string, error) {
	return m.configStore.List(ctx)
}

// WatchConfig returns a channel that receives configuration events for the specified key
func (m *Manager) WatchConfig(key string) <-chan ConfigEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan ConfigEvent, 10)
	watcherKey := fmt.Sprintf("%s:%d", key, time.Now().UnixNano())
	m.watchers[watcherKey] = ch

	return ch
}

// LoadRuntimeConfig loads the main runtime configuration
func (m *Manager) LoadRuntimeConfig(ctx context.Context) (*RuntimeConfig, error) {
	config := &RuntimeConfig{}
	
	// Try to load existing config
	err := m.LoadConfig(ctx, "runtime", config)
	if err != nil {
		// If config doesn't exist, return default config
		config = DefaultRuntimeConfig()
	}

	return config, nil
}

// SaveRuntimeConfig saves the main runtime configuration
func (m *Manager) SaveRuntimeConfig(ctx context.Context, config *RuntimeConfig) error {
	return m.SaveConfig(ctx, "runtime", config)
}

// notifyWatchers sends an event to all matching watchers
func (m *Manager) notifyWatchers(event ConfigEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for watcherKey, ch := range m.watchers {
		// Extract the key from the watcher key
		parts := strings.Split(watcherKey, ":")
		if len(parts) > 0 && (parts[0] == event.Key || parts[0] == "*") {
			select {
			case ch <- event:
			default:
				// Channel full, skip
			}
		}
	}
}

// DefaultRuntimeConfig returns the default runtime configuration
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		StateStorePath: "./state",
		EnableRecovery: true,
		LogLevel:       "info",
		DefaultAgentConfig: AgentDefaults{
			Resources: runtime.ResourceLimits{
				Memory: "512Mi",
				CPU:    "0.5",
			},
			HealthCheck: runtime.HealthCheckConfig{
				Interval:    30 * time.Second,
				Timeout:     5 * time.Second,
				Retries:     3,
				StartPeriod: 10 * time.Second,
			},
			RestartPolicy: runtime.RestartPolicyOnFailure,
			Environment:   make(map[string]string),
		},
		HealthCheck: HealthCheckConfig{
			CheckInterval:      30 * time.Second,
			DefaultTimeout:     5 * time.Second,
			MaxRetries:         3,
			UnhealthyThreshold: 3,
		},
		MessageBroker: MessageBrokerConfig{
			QueueSize:     1000,
			MessageTTL:    5 * time.Minute,
			RetryCount:    3,
			RetryInterval: 1 * time.Second,
		},
		ServiceDiscovery: ServiceDiscoveryConfig{
			TTL:               5 * time.Minute,
			CleanupInterval:   1 * time.Minute,
			EnableAutoCleanup: true,
		},
		Recovery: RecoveryConfig{
			MaxRetries:        3,
			RetryInterval:     30 * time.Second,
			BackoffMultiplier: 2.0,
			MaxRetryInterval:  5 * time.Minute,
		},
	}
}