package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/hollis-labs/tools/folio/internal/runtime"
)

func TestDefaultRuntime(t *testing.T) {
	// Create runtime with test configuration
	config := runtime.Config{
		StateStorePath: "./test_state",
		EnableRecovery: false, // Disable for simpler testing
	}

	rt, err := runtime.NewRuntime(config)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	ctx := context.Background()

	// Test Start
	if err := rt.Start(ctx); err != nil {
		t.Fatalf("Failed to start runtime: %v", err)
	}

	// Test agent creation
	agentConfig := runtime.AgentConfig{
		Image: "test:latest",
		Environment: map[string]string{
			"TEST_ENV": "test_value",
		},
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
		RestartPolicy:   runtime.RestartPolicyOnFailure,
		CommunicationID: "test-agent",
	}

	// Note: This will fail because we don't have Docker/test executor set up
	// But it tests the interface
	agentID, err := rt.StartAgent(ctx, agentConfig)
	if err == nil {
		t.Logf("Started agent: %s", agentID)

		// Test GetAgent
		agent, err := rt.GetAgent(ctx, agentID)
		if err != nil {
			t.Errorf("Failed to get agent: %v", err)
		} else {
			if agent.ID != agentID {
				t.Errorf("Agent ID mismatch: got %s, want %s", agent.ID, agentID)
			}
		}

		// Test ListAgents
		agents, err := rt.ListAgents(ctx)
		if err != nil {
			t.Errorf("Failed to list agents: %v", err)
		} else {
			found := false
			for _, a := range agents {
				if a.ID == agentID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Agent not found in list")
			}
		}

		// Test StopAgent
		if err := rt.StopAgent(ctx, agentID); err != nil {
			t.Errorf("Failed to stop agent: %v", err)
		}
	} else {
		t.Logf("Expected failure starting agent (no Docker): %v", err)
	}

	// Test messaging
	msg := runtime.Message{
		From:    "test-sender",
		To:      "test-receiver",
		Type:    runtime.MessageTypeNotification,
		Payload: map[string]interface{}{"test": "message"},
		TTL:     5 * time.Minute,
	}

	if err := rt.SendMessage(ctx, msg); err != nil {
		t.Errorf("Failed to send message: %v", err)
	}

	// Test service discovery
	serviceInfo := runtime.ServiceInfo{
		Name:     "test-service",
		AgentID:  "test-agent-id",
		Endpoint: "http://test:8080",
		Metadata: map[string]string{
			"version": "1.0.0",
		},
		TTL: 5 * time.Minute,
	}

	if err := rt.RegisterService(ctx, serviceInfo); err != nil {
		t.Errorf("Failed to register service: %v", err)
	}

	services, err := rt.DiscoverServices(ctx, "test-service")
	if err != nil {
		t.Errorf("Failed to discover services: %v", err)
	} else {
		if len(services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(services))
		} else {
			if services[0].Name != "test-service" {
				t.Errorf("Service name mismatch: got %s, want test-service", services[0].Name)
			}
		}
	}

	// Clean up
	if err := rt.Stop(ctx); err != nil {
		t.Errorf("Failed to stop runtime: %v", err)
	}
}

func TestAgentStates(t *testing.T) {
	states := []runtime.AgentState{
		runtime.AgentStateStarting,
		runtime.AgentStateRunning,
		runtime.AgentStateStopping,
		runtime.AgentStateStopped,
		runtime.AgentStateError,
		runtime.AgentStateRecovering,
	}

	for _, state := range states {
		if state == "" {
			t.Errorf("Empty agent state")
		}
	}
}

func TestMessageTypes(t *testing.T) {
	messageTypes := []runtime.MessageType{
		runtime.MessageTypeCommand,
		runtime.MessageTypeResponse,
		runtime.MessageTypeNotification,
		runtime.MessageTypeHeartbeat,
		runtime.MessageTypeDiscovery,
	}

	for _, msgType := range messageTypes {
		if msgType == "" {
			t.Errorf("Empty message type")
		}
	}
}

func TestRestartPolicies(t *testing.T) {
	policies := []runtime.RestartPolicy{
		runtime.RestartPolicyAlways,
		runtime.RestartPolicyOnFailure,
		runtime.RestartPolicyNever,
	}

	for _, policy := range policies {
		if policy == "" {
			t.Errorf("Empty restart policy")
		}
	}
}