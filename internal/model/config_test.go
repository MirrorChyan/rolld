package model

import (
	"os"
	"path/filepath"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	nodes := `apisix:
  admin: "http://127.0.0.1:9180"
  admin-key: "key"
  upstream-server:
    - srv: res-backend
      id: "1"
      health-check: "/health"
      port: "5432/tcp"
`
	if err := os.WriteFile(filepath.Join(dir, "nodes.yaml"), []byte(nodes), 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	C = GatewayConfig{}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if C.Admin != "http://127.0.0.1:9180" || C.AdminKey != "key" {
		t.Fatalf("unexpected gateway config: %+v", C)
	}
	want := Server{ID: "1", Srv: "res-backend", HealthCheck: "/health", Port: "5432/tcp"}
	if len(C.UpstreamServer) != 1 || C.UpstreamServer[0] != want {
		t.Fatalf("UpstreamServer = %+v, want [%+v]", C.UpstreamServer, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	chdir(t, t.TempDir())
	if err := Load(); err == nil {
		t.Fatal("Load() without nodes.yaml should fail")
	}
}
