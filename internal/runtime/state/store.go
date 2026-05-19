package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

// FileStateStore implements persistent state storage using the filesystem
type FileStateStore struct {
	basePath string
	mu       sync.RWMutex
}

// NewFileStateStore creates a new file-based state store
func NewFileStateStore(basePath string) (*FileStateStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	return &FileStateStore{
		basePath: basePath,
	}, nil
}

// Save persists agent state to disk
func (s *FileStateStore) Save(ctx context.Context, id runtime.AgentID, data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.getFilePath(id)
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Marshal data to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// Load retrieves agent state from disk
func (s *FileStateStore) Load(ctx context.Context, id runtime.AgentID) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.getFilePath(id)
	
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent state not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var agent runtime.Agent
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent data: %w", err)
	}

	return &agent, nil
}

// Delete removes agent state from disk
func (s *FileStateStore) Delete(ctx context.Context, id runtime.AgentID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.getFilePath(id)
	
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete state file: %w", err)
	}

	return nil
}

// List returns all agent IDs that have persisted state
func (s *FileStateStore) List(ctx context.Context) ([]runtime.AgentID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var agentIDs []runtime.AgentID
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			// Extract agent ID from filename (remove .json extension)
			agentID := runtime.AgentID(entry.Name()[:len(entry.Name())-5])
			agentIDs = append(agentIDs, agentID)
		}
	}

	return agentIDs, nil
}

// getFilePath returns the file path for an agent's state
func (s *FileStateStore) getFilePath(id runtime.AgentID) string {
	return filepath.Join(s.basePath, string(id)+".json")
}

// MemoryStateStore implements an in-memory state store for testing
type MemoryStateStore struct {
	data map[runtime.AgentID]interface{}
	mu   sync.RWMutex
}

// NewMemoryStateStore creates a new in-memory state store
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		data: make(map[runtime.AgentID]interface{}),
	}
}

// Save stores data in memory
func (s *MemoryStateStore) Save(ctx context.Context, id runtime.AgentID, data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep copy the data to avoid reference issues
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	var copied interface{}
	if err := json.Unmarshal(jsonData, &copied); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	s.data[id] = copied
	return nil
}

// Load retrieves data from memory
func (s *MemoryStateStore) Load(ctx context.Context, id runtime.AgentID) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.data[id]
	if !exists {
		return nil, fmt.Errorf("agent state not found: %s", id)
	}

	// Return a copy to avoid reference issues
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var agent runtime.Agent
	if err := json.Unmarshal(jsonData, &agent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent data: %w", err)
	}

	return &agent, nil
}

// Delete removes data from memory
func (s *MemoryStateStore) Delete(ctx context.Context, id runtime.AgentID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, id)
	return nil
}

// List returns all agent IDs
func (s *MemoryStateStore) List(ctx context.Context) ([]runtime.AgentID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agentIDs := make([]runtime.AgentID, 0, len(s.data))
	for id := range s.data {
		agentIDs = append(agentIDs, id)
	}

	return agentIDs, nil
}

// Clear removes all data (useful for testing)
func (s *MemoryStateStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[runtime.AgentID]interface{})
}