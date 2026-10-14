package config

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPrintPullProgress(t *testing.T) {
	ok := `{"status":"Pulling from mirrorchyan/res","id":"main"}
{"status":"Downloading","progressDetail":{"current":1,"total":2},"progress":"[=>  ]","id":"abc"}
{"status":"Status: Downloaded newer image for ghcr.io/mirrorchyan/res:main"}
`
	if err := printPullProgress(strings.NewReader(ok), &bytes.Buffer{}); err != nil {
		t.Fatalf("successful pull reported error: %v", err)
	}

	failed := `{"status":"Pulling from mirrorchyan/res","id":"main"}
{"errorDetail":{"message":"denied: denied"},"error":"denied: denied"}
`
	err := printPullProgress(strings.NewReader(failed), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("failed pull: err = %v, want denied", err)
	}
}

func TestLoadComposeConfigMissing(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if _, err := LoadComposeConfig(); err == nil {
		t.Fatal("LoadComposeConfig() without compose.yaml should fail")
	}
}
