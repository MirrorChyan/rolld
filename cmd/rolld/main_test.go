package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunWithoutConfig(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var stdout, stderr bytes.Buffer
	if code := run([]string{"up", "res-backend"}, &stdout, &stderr); code != 1 {
		t.Fatalf("run without nodes.yaml = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "nodes.yaml") {
		t.Errorf("stderr = %q, want it to mention nodes.yaml", stderr.String())
	}
}

func TestRunUsage(t *testing.T) {
	cases := []struct {
		args       []string
		code       int
		wantStdout bool
	}{
		{[]string{"help"}, 0, true},
		{[]string{"-h"}, 0, true},
		{[]string{"--help"}, 0, true},
		{[]string{"up"}, 2, false},
		{[]string{"prune"}, 2, false},
		{[]string{"deploy", "res-backend"}, 2, false},
		{[]string{"up", "res-backend", "extra"}, 2, false},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, &stdout, &stderr); code != c.code {
			t.Errorf("run(%v) = %d, want %d", c.args, code, c.code)
		}
		out := stderr.String()
		if c.wantStdout {
			out = stdout.String()
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("run(%v) did not print usage, stdout=%q stderr=%q", c.args, stdout.String(), stderr.String())
		}
	}
}
