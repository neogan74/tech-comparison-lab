package main

import "testing"

func TestValidateConfigAcceptsSupportedBrokers(t *testing.T) {
	t.Parallel()

	for _, broker := range []string{"kafka", "pulsar", "rabbitmq", "nats"} {
		broker := broker
		t.Run(broker, func(t *testing.T) {
			t.Parallel()
			if err := validateConfig(broker, "all", 1000, 100, 3, 3); err != nil {
				t.Fatalf("validateConfig(%q) returned error: %v", broker, err)
			}
		})
	}
}

func TestValidateConfigRejectsUnknownBroker(t *testing.T) {
	t.Parallel()

	if err := validateConfig("unknown", "all", 1000, 100, 3, 3); err == nil {
		t.Fatal("validateConfig accepted an unknown broker")
	}
}
