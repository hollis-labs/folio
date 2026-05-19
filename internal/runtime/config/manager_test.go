package config

import (
	"testing"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

func TestFileConfigStore(t *testing.T) {
	// Create temporary directory for testing
	tempDir := filepath.Join(os.TempDir(), "folio_config_test")
	defer os.RemoveAll(tempDir)

	store, err := NewFileConfigStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	ctx := context.Background()

	// Test saving and loading YAML config
	testConfig := &RuntimeConfig{
		StateStorePath: "./test_state",
		EnableRecovery: true,
		LogLevel:       "debug",
	}

	// Save config
	err = store.Save(ctx, "test_runtime", testConfig)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load config
	loadedConfig := &RuntimeConfig{}
	err = store.Load(ctx, "test_runtime", loadedConfig)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify config
	if loadedConfig.StateStorePath != testConfig.StateStorePath {
		t.Errorf("StateStorePath mismatch: got %s, want %s", 
			loadedConfig.StateStorePath, testConfig.StateStorePath)
	}
	if loadedConfig.EnableRecovery != testConfig.EnableRecovery {
		t.Errorf("EnableRecovery mismatch: got %v, want %v", 
			loadedConfig.EnableRecovery, testConfig.EnableRecovery)
	}
	if loadedConfig.LogLevel != testConfig.LogLevel {
		t.Errorf("LogLevel mismatch: got %s, want %s", 
			loadedConfig.LogLevel, testConfig.LogLevel)
	}

	// Test listing configs
	keys, err := store.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list configs: %v", err)
	}

	found := false
	for _, key := range keys {
		if key == "test_runtime" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("test_runtime config not found in list")
	}

	// Test deleting config
	err = store.Delete(ctx, "test_runtime")
	if err != nil {
		t.Fatalf("Failed to delete config: %v", err)
	}

	// Verify deletion
	err = store.Load(ctx, "test_runtime", &RuntimeConfig{})
	if err == nil {
		t.Errorf("Expected error loading deleted config, got nil")
	}
}

func TestConfigManager(t *testing.T) {
	// Create temporary directory for testing
	tempDir := filepath.Join(os.TempDir(), "folio_config_mgr_test")
	defer os.RemoveAll(tempDir)

	store, err := NewFileConfigStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	manager := NewManager(store)
	ctx := context.Background()

	// Start manager
	err = manager.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start config manager: %v", err)
	}

	// Test saving and loading through manager
	testConfig := &RuntimeConfig{
		StateStorePath: "./manager_test_state",
		EnableRecovery: false,
	}

	err = manager.SaveConfig(ctx, "manager_test", testConfig)
	if err != nil {
		t.Fatalf("Failed to save config through manager: %v", err)
	}

	loadedConfig := &RuntimeConfig{}
	err = manager.LoadConfig(ctx, "manager_test", loadedConfig)
	if err != nil {
		t.Fatalf("Failed to load config through manager: %v", err)
	}

	if loadedConfig.StateStorePath != testConfig.StateStorePath {
		t.Errorf("StateStorePath mismatch: got %s, want %s", 
			loadedConfig.StateStorePath, testConfig.StateStorePath)
	}

	// Test config watching
	watchCh := manager.WatchConfig("manager_test")
	
	// Update config
	testConfig.LogLevel = "warn"
	err = manager.SaveConfig(ctx, "manager_test", testConfig)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Check for event
	select {
	case event := <-watchCh:
		if event.Type != ConfigEventCreated {
			t.Errorf("Expected ConfigEventCreated, got %s", event.Type)
		}
		if event.Key != "manager_test" {
			t.Errorf("Expected key 'manager_test', got %s", event.Key)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Did not receive config event")
	}

	// Test runtime config
	runtimeConfig, err := manager.LoadRuntimeConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to load runtime config: %v", err)
	}

	// Should return default config since none exists
	defaultConfig := DefaultRuntimeConfig()
	if runtimeConfig.StateStorePath != defaultConfig.StateStorePath {
		t.Errorf("Default StateStorePath mismatch: got %s, want %s", 
			runtimeConfig.StateStorePath, defaultConfig.StateStorePath)
	}

	// Save runtime config
	runtimeConfig.LogLevel = "debug"
	err = manager.SaveRuntimeConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatalf("Failed to save runtime config: %v", err)
	}

	// Load again and verify
	loadedRuntimeConfig, err := manager.LoadRuntimeConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to load saved runtime config: %v", err)
	}

	if loadedRuntimeConfig.LogLevel != "debug" {
		t.Errorf("LogLevel mismatch: got %s, want debug", loadedRuntimeConfig.LogLevel)
	}

	// Stop manager
	err = manager.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop config manager: %v", err)
	}
}

func TestDefaultRuntimeConfig(t *testing.T) {
	config := DefaultRuntimeConfig()
	
	if config.StateStorePath == "" {
		t.Error("Default StateStorePath should not be empty")
	}
	
	if config.DefaultAgentConfig.Resources.Memory == "" {
		t.Error("Default agent memory should not be empty")
	}
	
	if config.DefaultAgentConfig.Resources.CPU == "" {
		t.Error("Default agent CPU should not be empty")
	}
	
	if config.HealthCheck.CheckInterval <= 0 {
		t.Error("Default health check interval should be positive")
	}
	
	if config.MessageBroker.QueueSize <= 0 {
		t.Error("Default message broker queue size should be positive")
	}
	
	if config.Recovery.MaxRetries <= 0 {
		t.Error("Default recovery max retries should be positive")
	}
}