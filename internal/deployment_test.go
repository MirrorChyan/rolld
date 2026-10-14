package internal

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

func TestServiceNamePattern(t *testing.T) {
	cases := []struct {
		srv, name string
		want      bool
	}{
		{"res-backend", "/res-backend-" + time.Now().Format(nameLayout), true},
		{"res-backend", "/res-backend-10.10-04.44.07", true},
		{"res-backend", "res-backend-10.10-04.44.07", true},
		{"res-backend", "/res-backend-admin-10.15-00.15.52", false},
		{"res", "/res-backend-10.15-00.15.52", false},
		{"cdk-backend", "/cdk-backend", false},
		{"user-frontend", "/user-frontend-10.10-04.43.32-old", false},
		{"a.b", "/axb-10.10-04.43.32", false},
	}
	for _, c := range cases {
		if got := serviceNamePattern(c.srv).MatchString(c.name); got != c.want {
			t.Errorf("serviceNamePattern(%q).MatchString(%q) = %v, want %v", c.srv, c.name, got, c.want)
		}
	}
}

func TestParseNodePorts(t *testing.T) {
	cases := []struct {
		nodes string
		want  []string
	}{
		{`{"docker.local:32283": 100}`, []string{"32283"}},
		{`[{"host": "docker.local", "port": 24575, "weight": 100}]`, []string{"24575"}},
		{`{}`, []string{}},
		{`[]`, []string{}},
	}
	for _, c := range cases {
		got, err := parseNodePorts(json.RawMessage(c.nodes))
		if err != nil {
			t.Fatalf("parseNodePorts(%s): %v", c.nodes, err)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("parseNodePorts(%s) = %v, want %v", c.nodes, got, c.want)
		}
	}
	if _, err := parseNodePorts(json.RawMessage(`"bad"`)); err == nil {
		t.Error("parseNodePorts of a string should fail")
	}
}

func containerOn(name, port string) types.ContainerJSON {
	bindings := nat.PortMap{}
	if port != "" {
		bindings["5432/tcp"] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: port}, {HostIP: "::", HostPort: port}}
	}
	return types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{
		Name:       name,
		HostConfig: &container.HostConfig{PortBindings: bindings},
	}}
}

func names(containers []types.ContainerJSON) []string {
	var out []string
	for _, c := range containers {
		out = append(out, c.Name)
	}
	return out
}

func TestHostPort(t *testing.T) {
	if port, err := hostPort(containerOn("/a", "32283")); err != nil || port != "32283" {
		t.Errorf("hostPort = %q, %v, want 32283", port, err)
	}
	// what earlier rolld versions stored, as seen on a live container
	if port, err := hostPort(containerOn("/a", "19719/tcp")); err != nil || port != "19719" {
		t.Errorf("hostPort of 19719/tcp = %q, %v, want 19719", port, err)
	}
	if _, err := hostPort(containerOn("/a", "")); err == nil {
		t.Error("hostPort without bindings should fail")
	}
	if _, err := hostPort(types.ContainerJSON{}); err == nil {
		t.Error("hostPort without host config should fail")
	}
}

func TestPruneTargets(t *testing.T) {
	newest, middle, oldest := containerOn("/s-3", "3"), containerOn("/s-2", "2"), containerOn("/s-1", "1")
	unknown := containerOn("/s-0", "")
	cases := []struct {
		name       string
		containers []types.ContainerJSON
		serving    []string
		want       []string
		wantErr    bool
	}{
		{"upstream on newest", []types.ContainerJSON{newest, middle, oldest}, []string{"3"}, []string{"/s-2", "/s-1"}, false},
		// a deploy whose upstream change did not land: keep the serving one and the newest
		{"upstream on an older one", []types.ContainerJSON{newest, middle, oldest}, []string{"2"}, []string{"/s-1"}, false},
		{"upstream on none", []types.ContainerJSON{newest, middle}, []string{"9"}, nil, true},
		{"upstream empty", []types.ContainerJSON{newest, middle}, []string{}, nil, true},
		{"unreadable port is kept", []types.ContainerJSON{newest, unknown, oldest}, []string{"3"}, []string{"/s-1"}, false},
		{"host ports stored with /tcp", []types.ContainerJSON{containerOn("/m-2", "19719/tcp"), containerOn("/m-1", "21599/tcp")}, []string{"19719"}, []string{"/m-1"}, false},
	}
	for _, c := range cases {
		got, err := pruneTargets(c.containers, c.serving)
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
		}
		if !slices.Equal(names(got), c.want) {
			t.Errorf("%s: targets = %v, want %v", c.name, names(got), c.want)
		}
	}
}
