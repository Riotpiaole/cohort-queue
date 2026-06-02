# Cohort backend — build, test, codegen, and image targets.
VERSION ?= 1.0.0
COORDINATOR_IMAGE ?= cohort-coordinator:v$(VERSION)
WORKER_IMAGE ?= cohort-worker:v$(VERSION)
GOBIN := $(shell go env GOPATH)/bin

.PHONY: help proto tools build test vet run-coordinator run-worker images docker-clean k8s-up k8s-down

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-18s %s\n", $$1, $$2}'

tools: ## Install codegen tools (protoc-gen-go, protoc-gen-go-grpc, buf) via go install
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go install github.com/bufbuild/buf/cmd/buf@v1.47.2

proto: ## Regenerate gRPC stubs into gen/ (commits the output)
	PATH="$(PATH):$(GOBIN)" buf generate

build: ## Build the cohort binary
	go build -o bin/cohort ./cmd/cohort

test: ## Run unit tests with the race detector
	go test -race ./...

vet: ## go vet
	go vet ./...

run-coordinator: ## Run coordinator locally (HTTP :8080, gRPC :9090)
	WORKSPACE_FOLDER=$(PWD) go run ./cmd/cohort --mode=coordinator

run-worker: ## Run 3 workers locally against localhost:9090
	go run ./cmd/cohort --mode=worker --coordinator-addr=localhost:9090 --workers=3

images: ## Build both v$(VERSION) images (same binary, two tags)
	docker build --build-arg VERSION=$(VERSION) -t $(COORDINATOR_IMAGE) .
	docker build --build-arg VERSION=$(VERSION) -t $(WORKER_IMAGE) .

docker-clean: ## Remove the built images
	-docker rmi $(COORDINATOR_IMAGE) $(WORKER_IMAGE)

k8s-up: ## Apply the k8s demo manifests
	kubectl apply -f deploy/k8s/

k8s-down: ## Delete the k8s demo manifests
	kubectl delete -f deploy/k8s/
