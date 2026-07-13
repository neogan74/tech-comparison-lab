package main

import "testing"

func TestValidateConfigAcceptsDragonfly(t *testing.T) {
	cfg := runConfig{
		op:         "all",
		count:      100,
		iterations: 10,
		pipeSize:   10,
		workers:    2,
	}

	if err := validateConfig("dragonfly", cfg); err != nil {
		t.Fatalf("validateConfig(dragonfly) returned error: %v", err)
	}
}
