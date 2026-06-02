# Cohort waiting list — one-command local stack on minikube.
#
#   make up      # cluster + build all images + deploy + open the UI  (the everything button)
#   make open    # just open the frontend in the browser
#   make down    # remove the workloads
#
# This is a thin orchestrator: image builds and k8s apply/delete are reused from
# backend/Makefile (targets `images`, `k8s-up`, `k8s-down`). The root only adds the
# minikube bootstrap, the frontend image, and the open/status helpers.
#
# Images are built INSIDE minikube's docker daemon (minikube docker-env) rather than
# `minikube image load`, because re-loading the same tag silently keeps the stale
# image — pods then run old code. Building in-daemon avoids that entirely.

VERSION      ?= 1.0.0
FRONTEND_IMG := cohort-frontend:v$(VERSION)
DEPLOYS      := deploy/cohort-coordinator deploy/cohort-frontend deploy/cohort-worker
LABEL        := app in (cohort-coordinator,cohort-worker,cohort-frontend)

.DEFAULT_GOAL := help
.PHONY: help up open url status logs cluster images deploy restart restart-wait down clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

up: cluster images deploy restart-wait ## Cluster + build + deploy everything, then open the UI
	@echo ""
	@echo "Stack is up. Opening the frontend (Ctrl-C to stop the tunnel)…"
	@$(MAKE) --no-print-directory open

cluster: ## Ensure a minikube cluster is running
	@minikube status >/dev/null 2>&1 || minikube start --driver=docker

images: cluster ## Build all 3 images inside minikube's docker daemon (reuses backend/Makefile)
	@echo "Building images inside minikube's docker daemon…"
	eval $$(minikube docker-env) && \
		$(MAKE) -C backend images VERSION=$(VERSION) && \
		docker build -t $(FRONTEND_IMG) .

deploy: cluster ## Apply the k8s manifests (reuses backend/Makefile k8s-up)
	$(MAKE) -C backend k8s-up

restart: ## Roll all deployments (pick up freshly built images)
	kubectl rollout restart $(DEPLOYS)

# Internal: restart then wait for every deployment to become available.
restart-wait: restart
	kubectl rollout status deploy/cohort-coordinator --timeout=180s
	kubectl rollout status deploy/cohort-frontend   --timeout=180s
	kubectl rollout status deploy/cohort-worker     --timeout=180s

open: ## Open the frontend in your browser (holds a tunnel; Ctrl-C to stop)
	minikube service cohort-frontend

url: ## Print the frontend URL (holds a tunnel on the docker driver)
	minikube service cohort-frontend --url

status: ## Show the deployed workloads
	kubectl get deploy,pods,svc -l '$(LABEL)'

logs: ## Tail worker + coordinator logs
	kubectl logs -f --prefix --max-log-requests=10 -l 'app in (cohort-worker,cohort-coordinator)'

down: ## Remove the workloads (reuses backend/Makefile k8s-down)
	$(MAKE) -C backend k8s-down

clean: down ## Delete everything, including the minikube cluster
	minikube delete
