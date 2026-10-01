package folio_test

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

func TestIntegration_ChatAppPreset_Render(t *testing.T) {
	for _, base := range []string{"/chat", "/conversation"} {
		t.Run(base, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "chat")
			svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version})
			if _, err := svc.New(service.NewOptions{
				PresetID: "chat-app", TargetDir: target,
				Inputs: map[string]any{"project_name": "smoke_chat", "github_owner": "hollis-labs", "base_path": base},
			}); err != nil {
				t.Fatal(err)
			}
			mf, err := manifest.Read(target)
			if err != nil {
				t.Fatal(err)
			}
			if mf.Presets[0].ID != "chat-app" {
				t.Fatalf("preset = %s, want chat-app", mf.Presets[0].ID)
			}
			if embed := readFile(t, target, "internal/webui/embed.go"); !strings.Contains(embed, `BasePath = "`+base+`"`) {
				t.Errorf("Go mount did not receive custom base path: %s", embed)
			}
			if vite := readFile(t, target, "frontend/vite.config.ts"); !strings.Contains(vite, `base: "`+base+`/"`) {
				t.Errorf("Vite did not receive custom base path: %s", vite)
			}
			if _, err := svc.Inspect(service.InspectOptions{TargetDir: target}); err != nil {
				t.Fatalf("inspect of generated chat app: %v", err)
			}
		})
	}
}

// Network builds and browser verification remain opt-in; the normal suite is offline.
func TestIntegration_ChatAppPreset_Frontend(t *testing.T) {
	if os.Getenv("FOLIO_FRONTEND_E2E") != "1" {
		t.Skip("set FOLIO_FRONTEND_E2E=1 to install and build the generated chat app")
	}
	target := filepath.Join(t.TempDir(), "chat")
	svc := service.New(service.Options{BundledFS: folio.BundledPresets, BundledRoot: "presets", UserDir: t.TempDir(), FolioVersion: folio.Version})
	if _, err := svc.New(service.NewOptions{
		PresetID: "chat-app", TargetDir: target,
		Inputs: map[string]any{"project_name": "smoke_chat", "github_owner": "hollis-labs"},
	}); err != nil {
		t.Fatal(err)
	}
	probeHome := t.TempDir()
	env := append(os.Environ(), "HOME="+probeHome, "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -modcacherw"))
	for _, args := range [][]string{{"install"}, {"run", "typecheck"}, {"run", "lint"}, {"run", "build"}} {
		cmd := exec.Command("npm", args...)
		cmd.Dir = filepath.Join(target, "frontend")
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		t.Logf("npm %v:\n%s", args, output)
		if err != nil {
			t.Fatalf("npm %v: %v", args, err)
		}
	}
	binary := filepath.Join(target, "chat-server")
	for _, args := range [][]string{{"mod", "tidy"}, {"build", "-o", binary, "./cmd/smoke_chat"}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = target
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, output)
		}
	}
	if os.Getenv("FOLIO_BROWSER_E2E") != "1" {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	server := exec.Command(binary)
	server.Env = append(env, "LISTEN_ADDR="+addr)
	var serverLog bytes.Buffer
	server.Stdout = &serverLog
	server.Stderr = &serverLog
	if startErr := server.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	defer func() {
		_ = server.Process.Kill()
		_ = server.Wait()
		t.Logf("generated server:\n%s", serverLog.String())
	}()
	cmd := exec.Command("node", "scripts/check-chat-consumer.cjs", "http://"+addr+"/chat/")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	t.Logf("browser verification:\n%s", output)
	if err != nil {
		t.Fatalf("browser verification: %v", err)
	}
}
