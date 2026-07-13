.DEFAULT_GOAL := help

SHELL := /usr/bin/env bash

EXPERIMENTS := \
	postgresql-vs-mongodb \
	mysql-vs-postgres \
	postgres-vs-cockroachdb \
	grpc-vs-rest \
	graphql-vs-rest \
	kong-vs-traefik \
	redis-vs-valkey \
	redis-vs-memcached \
	redis-vs-dragonfly \
	clickhouse-vs-postgresql \
	kafka-vs-rabbitmq \
	nats-vs-kafka \
	rabbitmq-vs-nats \
	k8s-vs-openshift \
	etcd-vs-consul \
	argocd-vs-fluxcd \
	prometheus-vs-victoriametrics

.PHONY: \
	help \
	list \
	collect-results \
	render-results \
	validate \
	smoke \
	validate-all \
	smoke-all \
	$(addprefix validate-,$(EXPERIMENTS)) \
	$(addprefix smoke-,$(EXPERIMENTS))

help:
	@echo "Repository commands"
	@echo ""
	@echo "  make list"
	@echo "  make collect-results"
	@echo "  make render-results"
	@echo "  make validate-all"
	@echo "  make smoke-all"
	@echo "  make validate EXP=<experiment>"
	@echo "  make smoke EXP=<experiment>"
	@echo ""
	@echo "Available experiments:"
	@for exp in $(EXPERIMENTS); do echo "  - $$exp"; done

list:
	@for exp in $(EXPERIMENTS); do echo "$$exp"; done

collect-results:
	bash "./scripts/collect-results.sh"

render-results:
	bash "./scripts/render-results.sh"

validate:
ifndef EXP
	$(error usage: make validate EXP=<experiment>)
endif
	@$(MAKE) validate-$(EXP)

smoke:
ifndef EXP
	$(error usage: make smoke EXP=<experiment>)
endif
	@$(MAKE) smoke-$(EXP)

validate-all: $(addprefix validate-,$(EXPERIMENTS))

smoke-all: $(addprefix smoke-,$(EXPERIMENTS))

define validate_rule
validate-$(1):
	@test -f "./scripts/validate-$(1).sh" || { echo "error: unknown experiment '$(1)'"; exit 1; }
	bash "./scripts/validate-$(1).sh"
endef

define smoke_rule
smoke-$(1):
	@test -f "./scripts/smoke-$(1).sh" || { echo "error: unknown experiment '$(1)'"; exit 1; }
	bash "./scripts/smoke-$(1).sh"
endef

$(foreach exp,$(EXPERIMENTS),$(eval $(call validate_rule,$(exp))))
$(foreach exp,$(EXPERIMENTS),$(eval $(call smoke_rule,$(exp))))
