package main

import (
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	valid := config{platform: "kubernetes", rounds: 1, replicas: 2, timeout: time.Second, namespace: "bench"}
	if err := validateConfig(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []config{
		{platform: "mesos", rounds: 1, replicas: 2, timeout: time.Second, namespace: "bench"},
		{platform: "nomad", rounds: 0, replicas: 2, timeout: time.Second, namespace: "bench"},
		{platform: "nomad", rounds: 1, replicas: 1, timeout: time.Second, namespace: "bench"},
		{platform: "nomad", rounds: 1, replicas: 2, timeout: 0, namespace: "bench"},
	}
	for index, value := range tests {
		if err := validateConfig(value); err == nil {
			t.Errorf("invalid config %d accepted", index)
		}
	}
}
