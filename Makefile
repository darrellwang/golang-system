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

docker-up: ## Start the full system via Docker Compose
	docker compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 5

docker-down: ## Stop and remove all containers
	docker compose down

docker-logs: ## Tail logs of the two application services
	docker compose logs -f golang-app golang-app2

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
