package main

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func composeServices(t *testing.T, overlay bool) map[string]struct {
	Environment map[string]string
	Ports       []struct {
		HostIP string `json:"host_ip"`
		Target int
	}
} {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker CLI unavailable for Compose contract")
	}
	args := []string{"compose", "-f", "../deploy/compose/microservices.yml"}
	if overlay {
		args = append(args, "-f", "../deploy/compose/microservices-tracing.yml")
	}
	args = append(args, "config", "--format", "json")
	data, err := exec.Command("docker", args...).Output()
	if err != nil {
		t.Fatalf("compose config: %v", err)
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]string
			Ports       []struct {
				HostIP string `json:"host_ip"`
				Target int
			}
		}
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config.Services
}

func TestTracingOverlayPreservesBusinessEnvironmentAndPrivateCollector(t *testing.T) {
	base, traced := composeServices(t, false), composeServices(t, true)
	if _, found := base["jaeger"]; found {
		t.Fatal("tracing is not optional")
	}
	for _, name := range []string{"platform", "judge-api", "worker", "executor", "executor-b"} {
		if base[name].Environment["OTEL_EXPORTER_OTLP_ENDPOINT"] != "" {
			t.Fatalf("base exports traces: %s", name)
		}
		if traced[name].Environment["OTEL_EXPORTER_OTLP_ENDPOINT"] != "http://jaeger:4318" {
			t.Fatalf("no exporter: %s", name)
		}
		for key, value := range base[name].Environment {
			if traced[name].Environment[key] != value {
				t.Fatalf("overlay changed business env %s.%s", name, key)
			}
		}
	}
	ports := traced["jaeger"].Ports
	if len(ports) != 1 || ports[0].HostIP != "127.0.0.1" || ports[0].Target != 16686 {
		t.Fatalf("collector exposed or UI not loopback: %+v", ports)
	}
}
