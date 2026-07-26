.PHONY: help all build test lint tidy docker-up docker-down docker-logs migrate migrate-up migrate-down migrate-version migrate-create clean generate helm-deps helm-template helm-install helm-uninstall

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

all: lint test build ## Lint + test + build

build: ## Build all service binaries into <service>/server
	cd golang-commons && go build ./...
	cd golang-events && go build ./...
	cd golang-app && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server
	cd golang-app2 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

build-migrate: ## Build migration CLI tool
	cd golang-app && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../migrate-cli ./cmd/migrate

test: ## Run tests with race detection
	go test -race ./golang-commons/... ./golang-events/... ./golang-app/... ./golang-app2/...

lint: ## Run golangci-lint on all modules
	golangci-lint run ./golang-commons/... ./golang-events/... ./golang-app/... ./golang-app2/...

tidy: ## Run go mod tidy on all modules
	cd golang-commons && go mod tidy
	cd golang-events && go mod tidy
	cd golang-app && go mod tidy
	cd golang-app2 && go mod tidy

docker-up: ## Start infrastructure (PostgreSQL x2 + NATS) via Docker Compose
	docker compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 5

docker-down: ## Stop and remove all containers
	docker compose down

docker-logs: ## Tail logs of the infrastructure services (PostgreSQL + NATS)
	docker compose logs -f postgres-app postgres-app2 nats

migrate: migrate-up ## Run database migrations (default: up)

migrate-up: ## Run all available migrations on both databases
	@echo "Running migrations for golang-app..."
	@command -v migrate >/dev/null 2>&1 || go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	migrate -path golang-app/db/migrations -database "postgres://app_user:app_password@localhost:5432/app_db?sslmode=disable" up
	@echo "Running migrations for golang-app2..."
	migrate -path golang-app2/db/migrations -database "postgres://app2_user:app2_password@localhost:5433/app2_db?sslmode=disable" up

migrate-down: ## Roll back last migration (use COUNT=n to roll back more)
	@COUNT=$(or $(COUNT),1); \
	echo "Rolling back $$COUNT migration(s) for golang-app..."; \
	migrate -path golang-app/db/migrations -database "postgres://app_user:app_password@localhost:5432/app_db?sslmode=disable" down $$COUNT; \
	echo "Rolling back $$COUNT migration(s) for golang-app2..."; \
	migrate -path golang-app2/db/migrations -database "postgres://app2_user:app2_password@localhost:5433/app2_db?sslmode=disable" down $$COUNT

migrate-version: ## Show current migration version
	@echo "golang-app:"; \
	migrate -path golang-app/db/migrations -database "postgres://app_user:app_password@localhost:5432/app_db?sslmode=disable" version; \
	echo "golang-app2:"; \
	migrate -path golang-app2/db/migrations -database "postgres://app2_user:app2_password@localhost:5433/app2_db?sslmode=disable" version

migrate-create: ## Create a new migration file (usage: make migrate-create NAME=create_new_table)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required"; \
		echo "Usage: make migrate-create NAME=create_new_table"; \
		exit 1; \
	fi
	@command -v migrate >/dev/null 2>&1 || go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	migrate create -ext sql -dir golang-app/db/migrations -seq $(NAME)
	migrate create -ext sql -dir golang-app2/db/migrations -seq $(NAME)

clean: ## Remove build artifacts
	rm -rf golang-app/server golang-app2/server dist/ *.tmp migrate-cli

generate: ## Run go generate in all services
	cd golang-app && go generate ./...
	cd golang-app2 && go generate ./...

helm-deps: ## Update Helm Chart dependencies
	cd golang-system-chart && helm dependency update

helm-template: ## Render Helm templates locally
	cd golang-system-chart && helm template golang-system .

helm-install: ## Deploy to Kubernetes
	cd golang-system-chart && helm install golang-system . --namespace golang-system --create-namespace

helm-uninstall: ## Uninstall from Kubernetes
	helm uninstall golang-system -n golang-system
	kubectl delete namespace golang-system

# =============================================================================
# Istio 服务治理命令 (新增)
# =============================================================================

.PHONY: istio-install istio-uninstall istio-deploy istio-governance istio-verify istio-test istio-cleanup istio-dashboard

istio-install: ## Install Istio service mesh
	@echo "Installing Istio..."
	istioctl install -f istio-config/istio-config.yaml -y
	kubectl label namespace default istio-injection=enabled --overwrite
	@echo "Istio installed successfully"

istio-uninstall: ## Uninstall Istio service mesh
	@echo "Uninstalling Istio..."
	istioctl uninstall -y --purge
	kubectl delete namespace istio-system --ignore-not-found=true
	@echo "Istio uninstalled successfully"

istio-deploy: ## Deploy microservices with Istio sidecar
	@echo "Deploying microservices..."
	cd golang-system-chart && helm dependency update
	helm upgrade --install golang-system ./golang-system-chart \
		--namespace default \
		--create-namespace \
		--values golang-system-chart/values.yaml \
		--wait \
		--timeout 10m
	@echo "Microservices deployed successfully"

istio-governance: ## Apply service governance configurations (circuit breaking, rate limiting, API gateway)
	@echo "Applying service governance configurations..."
	kubectl apply -f istio-config/destination-rule-circuit-breaking.yaml
	kubectl apply -f istio-config/envoy-filter-rate-limiting.yaml
	kubectl apply -f istio-config/gateway.yaml
	kubectl apply -f istio-config/virtual-service-gateway.yaml
	@echo "Service governance configurations applied"

istio-verify: ## Verify Istio deployment and configurations
	@echo "Verifying Istio deployment..."
	kubectl get pods -n istio-system
	kubectl get pods -n default
	kubectl get gateway,virtualservice,destinationrule,envoyfilter -n default
	@echo "Verification completed"

istio-test: ## Run service governance tests
	@echo "Running tests..."
	@echo "Deploying fortio for load testing..."
	kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/sample-client/fortio-deploy.yaml || true
	kubectl rollout status deployment/fortio
	@echo "Testing service discovery..."
	kubectl exec fortio -- curl -s http://golang-app:8080/rest/v1/items | head -20
	@echo "Testing API gateway..."
	export INGRESS_IP=$$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "127.0.0.1"); \
	curl -s http://$$INGRESS_IP/rest/v1/items -H "Host: api.example.com" | head -20
	@echo "Tests completed"

istio-cleanup: ## Clean up Istio configurations and microservices
	@echo "Cleaning up..."
	kubectl delete -f istio-config/gateway.yaml --ignore-not-found=true
	kubectl delete -f istio-config/virtual-service-gateway.yaml --ignore-not-found=true
	kubectl delete -f istio-config/destination-rule-circuit-breaking.yaml --ignore-not-found=true
	kubectl delete -f istio-config/envoy-filter-rate-limiting.yaml --ignore-not-found=true
	helm uninstall golang-system -n default --ignore-not-found=true
	@echo "Cleanup completed"

istio-dashboard: ## Open Istio dashboards (Kiali, Prometheus, Grafana, Jaeger)
	@echo "Opening Istio dashboards..."
	@echo "Kiali: http://localhost:20001"
	@echo "Prometheus: http://localhost:9090"
	@echo "Grafana: http://localhost:3000"
	@echo "Jaeger: http://localhost:16686"
	istioctl dashboard kiali &
	istioctl dashboard prometheus &
	istioctl dashboard grafana &
	istioctl dashboard jaeger &

istio-all: istio-install istio-deploy istio-governance ## Complete Istio setup (install + deploy + governance)
	@echo "Complete Istio setup finished!"
	@make istio-verify

# =============================================================================
# Helm Chart 更新 (支持 Istio)
# =============================================================================

helm-istio: ## Deploy to Kubernetes with Istio enabled
	cd golang-system-chart && helm upgrade --install golang-system . \
		--namespace default \
		--create-namespace \
		--set istio.enabled=true \
		--set istio.injection.enabled=true \
		--wait \
		--timeout 10m
