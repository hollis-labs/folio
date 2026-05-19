package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// DockerExecutor implements agent execution using Docker
type DockerExecutor struct {
	containers map[AgentID]string // AgentID -> container ID
	mu         sync.RWMutex
}

// Start starts an agent using Docker
func (e *DockerExecutor) Start(ctx context.Context, agent *Agent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.containers == nil {
		e.containers = make(map[AgentID]string)
	}

	// Check if agent is already running
	if containerID, exists := e.containers[agent.ID]; exists {
		// Check if container is actually running
		if e.isContainerRunning(containerID) {
			return fmt.Errorf("agent %s is already running", agent.ID)
		}
		// Clean up stale container reference
		delete(e.containers, agent.ID)
	}

	// Build docker run command
	args := []string{"run", "-d", "--name", string(agent.ID)}

	// Add environment variables
	for key, value := range agent.Config.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add resource limits
	if agent.Config.Resources.Memory != "" {
		args = append(args, "--memory", agent.Config.Resources.Memory)
	}
	if agent.Config.Resources.CPU != "" {
		args = append(args, "--cpus", agent.Config.Resources.CPU)
	}

	// Add labels for identification
	args = append(args, "--label", fmt.Sprintf("folio.agent.id=%s", agent.ID))
	args = append(args, "--label", fmt.Sprintf("folio.agent.name=%s", agent.Name))

	// Add image
	args = append(args, agent.Config.Image)

	// Execute docker command
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start docker container: %w, output: %s", err, string(output))
	}

	// Get container ID from output
	containerID := strings.TrimSpace(string(output))
	e.containers[agent.ID] = containerID

	return nil
}

// Stop stops an agent's Docker container
func (e *DockerExecutor) Stop(ctx context.Context, id AgentID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	containerID, exists := e.containers[id]
	if !exists {
		return fmt.Errorf("agent %s is not running", id)
	}

	// Stop the container
	cmd := exec.CommandContext(ctx, "docker", "stop", containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop docker container: %w", err)
	}

	// Remove the container
	cmd = exec.CommandContext(ctx, "docker", "rm", containerID)
	if err := cmd.Run(); err != nil {
		// Log warning but don't fail
		fmt.Printf("Warning: failed to remove container %s: %v\n", containerID, err)
	}

	delete(e.containers, id)
	return nil
}

// Restart restarts an agent's Docker container
func (e *DockerExecutor) Restart(ctx context.Context, id AgentID) error {
	e.mu.RLock()
	containerID, exists := e.containers[id]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s is not running", id)
	}

	// Restart the container
	cmd := exec.CommandContext(ctx, "docker", "restart", containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart docker container: %w", err)
	}

	return nil
}

// GetStatus returns the current status of an agent
func (e *DockerExecutor) GetStatus(ctx context.Context, id AgentID) (AgentState, error) {
	e.mu.RLock()
	containerID, exists := e.containers[id]
	e.mu.RUnlock()

	if !exists {
		return AgentStateStopped, nil
	}

	// Check container status
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return AgentStateError, fmt.Errorf("failed to inspect container: %w", err)
	}

	status := strings.TrimSpace(string(output))
	switch status {
	case "running":
		return AgentStateRunning, nil
	case "exited":
		return AgentStateStopped, nil
	case "restarting":
		return AgentStateRecovering, nil
	case "created":
		return AgentStateStarting, nil
	default:
		return AgentStateError, nil
	}
}

// isContainerRunning checks if a container is currently running
func (e *DockerExecutor) isContainerRunning(containerID string) bool {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "true"
}

// ProcessExecutor implements agent execution using OS processes
type ProcessExecutor struct {
	processes map[AgentID]*exec.Cmd
	mu        sync.RWMutex
}

// Start starts an agent as an OS process
func (e *ProcessExecutor) Start(ctx context.Context, agent *Agent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.processes == nil {
		e.processes = make(map[AgentID]*exec.Cmd)
	}

	// Check if agent is already running
	if cmd, exists := e.processes[agent.ID]; exists {
		if cmd.Process != nil && cmd.ProcessState == nil {
			return fmt.Errorf("agent %s is already running", agent.ID)
		}
		delete(e.processes, agent.ID)
	}

	// For process executor, the "image" field should contain the command to run
	commandParts := strings.Fields(agent.Config.Image)
	if len(commandParts) == 0 {
		return fmt.Errorf("no command specified in image field")
	}

	cmd := exec.CommandContext(ctx, commandParts[0], commandParts[1:]...)

	// Set environment variables
	for key, value := range agent.Config.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	e.processes[agent.ID] = cmd
	return nil
}

// Stop stops an agent process
func (e *ProcessExecutor) Stop(ctx context.Context, id AgentID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd, exists := e.processes[id]
	if !exists {
		return fmt.Errorf("agent %s is not running", id)
	}

	if cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		cmd.Wait() // Wait for process to finish
	}

	delete(e.processes, id)
	return nil
}

// Restart restarts an agent process
func (e *ProcessExecutor) Restart(ctx context.Context, id AgentID) error {
	// For process executor, restart means stop and start
	// We need the original agent config, which we don't have here
	// This would need to be enhanced to store agent configs
	return fmt.Errorf("restart not implemented for process executor")
}

// GetStatus returns the current status of an agent process
func (e *ProcessExecutor) GetStatus(ctx context.Context, id AgentID) (AgentState, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cmd, exists := e.processes[id]
	if !exists {
		return AgentStateStopped, nil
	}

	if cmd.Process == nil {
		return AgentStateError, nil
	}

	if cmd.ProcessState == nil {
		return AgentStateRunning, nil
	}

	if cmd.ProcessState.Exited() {
		return AgentStateStopped, nil
	}

	return AgentStateRunning, nil
}