# Golang System — Multi-Microservice Platform

A production-oriented multi-service Go microservice platform template following the **Hexagonal Architecture (Ports & Adapters)** and **Event-Driven** design patterns.

> 本文档的简体中文版本请见 [README_CN.md](./README_CN.md)。

## Table of Contents

1. [System Architecture](#1-system-architecture)
2. [Project Structure](#2-project-structure)
3. [Sub-Project Details](#3-sub-project-details)
4. [Service Communication](#4-service-communication)
5. [NATS Event-Driven Messaging](#5-nats-event-driven-messaging)
6. [API Reference](#6-api-reference)
7. [Local Development Guide](#7-local-development-guide)
8. [Deployment Guide](#8-deployment-guide)
9. [CI/CD Pipelines](#9-cicd-pipelines)
10. [Configuration Reference](#10-configuration-reference)
11. [Command Reference](#11-command-reference)

---

## 1. System Architecture

```text
                      ┌────────────────┐
                      │   Client /      │
                      │   API Gateway   │
                      └───────┬────────┘
                              │ REST
              ┌───────────────┴───────────────┐
              ▼                               ▼
      ┌───────────────┐               ┌───────────────┐
      │   golang-app   │ ─── HTTP ───► │   golang-app2  │
      │   (Item Svc)   │ ◄── NATS ──  │   (Order Svc)  │
      │    :8080       │               │    :8081       │
      └───────┬───────┘               └───────┬───────┘
              │                               │
   ┌──────────┴─────┐                ┌─────────┴──────┐
   │   PostgreSQL    │                │   PostgreSQL    │
   │    (app_db)     │                │   (app2_db)     │
   │   Port: 5432    │                │   Port: 5433    │
   └─────────────────┘                └────────────────┘
                       │                ▲
                       └──────┬─────────┘
                              ▼
                  ┌──────────────────────┐
                  │        NATS           │
                  │  JetStream :4222      │
                  │  Monitoring  :8222    │
                  └──────────────────────┘
```

- The **Client / API Gateway** invokes item or order services via REST.
- **golang-app2 (Order)** synchronously calls **golang-app (Item)** over HTTP to validate that every referenced item exists when an order is created.
- Both services use **NATS** for asynchronous Pub/Sub domain events, decoupling the integration.
- NATS queue groups ensure each event is handled by only one instance, preventing duplicate processing.

### Communication Pattern Overview

| Mode | Direction | Description |
|------|-----------|-------------|
| HTTP REST       | app2 → app        | Synchronous item validation when creating an order |
| NATS Pub/Sub    | app → app2        | Item deletion triggers automatic cancellation of pending orders |
| NATS Pub/Sub    | app2 → app        | Order creation informs item service to track item references |
| NATS Req-Reply  | app2 → app        | Synchronous item lookup over NATS (HTTP alternative) |

---

## 2. Project Structure

```text
golang-system/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml            # CI pipeline: lint → test → build → Docker image
│   │   ├── cd.yml            # CD pipeline: dev → staging → production
│   │   └── release.yml       # Release pipeline: multi-arch binaries + images + Helm Chart
│   └── dependabot.yml        # Automated dependency updates
│
├── golang-commons/             # [Shared] Common reusable packages
│   ├── apperror/errors.go     # Structured error types and error codes
│   ├── database/postgres.go   # DB config, DSN builder, health check
│   ├── httpclient/client.go   # Inter-service HTTP client (auto header propagation)
│   ├── httputil/
│   │   ├── response.go        # JSON response helpers (OK, Created, Error, NoContent)
│   │   └── middleware.go      # Middleware: request ID, logging, auth, recovery, body size, content type
│   ├── logger/logger.go       # Zerolog setup (DEV console / PROD JSON)
│   ├── messaging/nats.go      # NATS publisher interface, noop, connection with retry, PROD JSON
│   └── go.mod
│
├── golang-events/              # [Shared] Event contract definitions
│   ├── events.go              # Event[T] envelope, NATS subject constants, payload structs
│   └── go.mod
│
├── golang-app/                 # [Service] Item microservice (:8080)
│   ├── cmd/server/main.go     # Entry point, graceful shutdown
│   ├── internal/
│   │   ├── app/app.go         # Composition root (dependency injection)
│   │   ├── config/config.go   # Env-var-based config
│   │   ├── domain/models.go   # Item entity, DTOs, pagination
│   │   ├── service/
│   │   │   ├── service.go     # Port interfaces (ItemRepository, EventPublisher)
│   │   │   ├── item_service.go     # Business logic
│   │   │   └── item_service_test.go # Unit tests (with mocks)
│   │   ├── ports/
│   │   │   └── http/event/handler.go  # REST handlers, routes, middleware, responses
│   │   └── adapter/
│   │       ├── repository/postgres.go # PostgreSQL implementation
│   │       └── messaging/nats.go      # NATS publisher implementation
│   ├── db/migrations/         # SQL database migrations
│   ├── api/openapi.yaml       # OpenAPI 3.0 specification
│   ├── helm/                  # Service-level Helm Chart
│   └── Dockerfile
│
├── golang-app2/                # [Service] Order microservice (:8081)
│   ├── cmd/server/main.go     # Entry point, graceful shutdown
│   ├── internal/
│   │   ├── app/app.go         # Composition root
│   │   ├── config/config.go   # Config (incl. ITEM_SERVICE_URL for inter-service call)
│   │   ├── domain/models.go   # Order, OrderItem entities, DTOs, ItemView
│   │   ├── client/item_client.go     # Item service HTTP client (GetByID, List)
│   │   ├── service/
│   │   │   ├── service.go     # Port interfaces (OrderRepository, ItemServiceClient)
│   │   │   ├── order_service.go      # Business logic with cross-service validation
│   │   │   └── order_service_test.go # Unit tests
│   │   ├── ports/
│   │   │   └── http/event/handler.go  # REST handlers, routes
│   │   └── adapter/
│   │       ├── repository/postgres.go # PostgreSQL implementation
│   │       └── messaging/nats.go      # NATS subscriber (item.deleted, item.updated)
│   ├── db/migrations/
│   ├── api/openapi.yaml
│   ├── helm/
│   └── Dockerfile
│
├── golang-system-chart/        # [Helm] Total Helm Chart (deploys entire system)
│   ├── Chart.yaml
│   ├── values.yaml            # Depends on: two apps + PostgreSQL + NATS
│   └── templates/NOTES.txt
│
├── docker-compose.yml          # Local full-stack startup
├── go.work                     # Go Workspace for multi-module dev
├── go.work.sum
├── Makefile                    # System-level build/test/deploy commands
├── .gitignore
├── README.md                   # English documentation (this file)
└── README_CN.md                # 中文文档
```

---

## 3. Sub-Project Details

### 3.1 golang-commons (Shared Library)

Common packages reused by all services. **Depends on no business service modules.**

| Package | Purpose | Key types/functions |
|---------|---------|---------------------|
| `apperror`   | Structured application errors         | `AppError`, `BadInput()`, `NotFound()`, `Internal()`, `Conflict()` |
| `database`   | PostgreSQL helpers                    | `Config`, `DSN()`, `NewHealthChecker()` |
| `httpclient` | Inter-service HTTP client             | `Client`, `New()`, `Get()`, `Post()`, `Put()`, `Delete()`, `IsNotFound()` |
| `httputil`   | HTTP response helpers                 | `RequestIDMiddleware`, `Error()`, `Created()`, `NoContent()` |
|             | Middleware                            | `MaxBodySizeMiddleware`, `ContentTypeMiddleware`, `LoggingMiddleware`, `AuthContextMiddleware`, `RecoveryMiddleware` |
| `logger`     | Zerolog initialization                | `Setup(level, mode)`, `DEV → console`, `PROD → JSON` |
| `messaging`  | NATS wrappers                         | `Publisher` (interface), `NatsPublisher` (impl), `NoopPublisher` (test), `Connect()` (with retry) |

### 3.2 golang-events (Shared Event Model)

Defines the **event contracts** shared between all services.

```go
// Generic event envelope
type Event[T any] struct {
    ID        string    `json:"id"`        // Unique event ID
    Type      string    `json:"type"`      // Event type
    Source    string    `json:"source"`    // Source service
    Timestamp time.Time `json:"timestamp"` // Event time
    Data      T         `json:"data"`      // Event payload
}
```

### NATS Subject Definitions

| Subject              | Direction  | Mode         | Payload |
|----------------------|------------|--------------|---------|
| `item.created`       | app → all  | Pub/Sub      | `ItemCreatedData{ID, Name, Description, Status}` |
| `item.updated`       | app → all  | Pub/Sub      | `ItemUpdatedData{ID, Name, Description, Status}` |
| `item.deleted`       | app → all  | Pub/Sub      | `ItemDeletedData{ID}` |
| `item.get_by_id`     | app2 → app | Req-Reply    | `GetItemRequest{ID}` → `GetItemResponse{ID, Name, Description, Status}` |
| `order.created`      | app2 → all | Pub/Sub      | `OrderCreatedData{ID, Items[], Total, Status}` |
| `order.updated`      | app2 → all | Pub/Sub      | `OrderUpdatedData{ID, Items[], Total, Status}` |
| `order.canceled`     | app2 → all | Pub/Sub      | `OrderCanceledData{ID, Reason}` |

### 3.3 golang-app (Item Service)

| Dimension     | Description |
|---------------|-------------|
| **Port**           | 8080 |
| **Database**       | PostgreSQL (`golang-app`) |
| **Architecture**   | Hexagonal (Ports & Adapters) |
| **API prefix**     | `/rest/v1/items` |
| **Domain**         | Item CRUD with status, tags, pagination |
| **Publishes**      | `item.created`, `item.updated`, `item.deleted` |
| **Subscribes**     | `order.created` (track item references), `item.get_by_id` (req-reply) |

### 3.4 golang-app2 (Order Service)

| Dimension     | Description |
|---------------|-------------|
| **Port**           | 8081 |
| **Database**       | PostgreSQL (`golang-app2`) |
| **Architecture**   | Hexagonal (Ports & Adapters) |
| **API prefix**     | `/rest/v1/orders` |
| **Domain**         | Orders (with line items), state transitions, cancellation |
| **Inter-service**  | HTTP call to item service to validate existence before order creation |
| **Publishes**      | `order.created`, `order.updated`, `order.canceled` |
| **Subscribes**     | `item.deleted` (auto-cancel affected pending orders), `item.updated` (visibility/cache) |

### 3.5 golang-system-chart (Total Helm Chart)

Deploys the entire system in a single Helm release:

- `golang-app` and `golang-app2` (business application services)
- PostgreSQL x 2 (via Bitnami Chart, one independent instance per service)
- NATS (with JetStream, via the official NATS Chart)

---

## 4. Service Communication

### 4.1 HTTP — Synchronous Item Validation

When `golang-app2` creates an order, it synchronously calls `golang-app` via HTTP to validate every referenced item:

```go
// internal/client/item_client.go
func (c *ItemClient) GetByID(ctx context.Context, id string) (*domain.ItemView, error) {
    var item domain.ItemView
    if err := c.client.Get(ctx, fmt.Sprintf("/rest/v1/items/%s", id), &item); err != nil {
        return nil, err
    }
    return &item, nil
}
```

### `httpclient.Client` Features

- **Request header propagation**: automatically forwards `X-Request-ID`, `X-Organization-ID`, `X-User-ID` from upstream to downstream services
- **Structured errors**: non-2xx responses return `ServiceError` containing the status code and response body
- **Configurable timeout**: default 30s, customizable via `httpclient.WithTimeout()`
- **Request logging**: records call duration, target service, and response status

### 4.2 NATS — Asynchronous Event-Driven

Both services publish domain events and subscribe to each other's events.

#### Flow 1: Create Order — Item Reference Tracking

```text
golang-app2 (Order)                  NATS              golang-app (Item)
        │                              │                       │
        │ ─ order.created ────────────►│                       │
        │    {id, items[], total,      │ ─ order.created ────► │
        │     status}                  │                       │
        │                              │                       │ handleOrderCreated()
        │                              │                       │   - Track which items
        │                              │                       │     are referenced
```

#### Flow 2: Delete Item — Auto-cancel Orders

```text
golang-app (Item)                    NATS              golang-app2 (Order)
        │                              │                       │
        │ ─ item.deleted ─────────────►│                       │
        │    {id: "abc-123"}           │ ─ item.deleted ─────► │
        │                              │                       │
        │                              │                       │ handleItemDeleted()
        │                              │                       │   - Find pending orders
        │                              │                       │     referencing item
        │                              │                       │   - Cancel all of them
```

#### Flow 3: NATS Request-Reply (Synchronous Query via NATS)

```text
golang-app2 (Order)                  NATS              golang-app (Item)
        │                              │                       │
        │ ─ item.get_by_id (req) ─────►│                       │
        │    {id: "abc-123"}           │ ─ item.get_by_id ───► │
        │                              │                       │
        │                              │                       │ handleGetItemByID()
        │                              │                       │   - Query database
        │ ◄── GetItemResponse ──────── │ ◄── response ────────│
        │     {id, name, description,  │                       │
        │      status}                 │                       │
```

---

## 5. NATS Event-Driven Messaging

### 5.1 Event Envelope

All domain events use the standard envelope defined in `golang-events`:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "item.created",
  "source": "golang-app",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "id": "...",
    "name": "widget A",
    "description": "...",
    "status": "active"
  }
}
```

### 5.2 Queue Groups

Both services use NATS **queue groups** for load balancing: when multiple instances of the same service run, only one instance per group receives each message, preventing duplicate processing.

| Service      | Queue Group         | Purpose |
|--------------|---------------------|---------|
| `golang-app`  | `golang-app-queue`  | Ensure each event is processed by only one instance |
| `golang-app2` | `golang-app2-queue` | Same — prevent duplicate order cancellation |

### 5.3 Event Handlers Summary

**Events subscribed by golang-app:**

| Subject                  | Handler                | Business logic |
|--------------------------|------------------------|----------------|
| `order.created`          | `handleOrderCreated()` | Record which items are referenced by new orders |
| `item.get_by_id`         | `handleGetItemByID()`  | Request-Reply: look up an item, return its details |
| `item.external.create`   | `handleExternalCreate()` | Receive item creation events from external systems |

**Events subscribed by golang-app2:**

| Subject         | Handler                | Business logic |
|-----------------|------------------------|----------------|
| `item.deleted`  | `handleItemDeleted()`  | Find pending orders referencing deleted items and auto-cancel them |
| `item.updated`  | `handleItemUpdated()`  | Record item change log for visibility and cache invalidation |

---

## 6. API Reference

### 6.1 golang-app — Item API (`:8080`)

| Method  | Path | Description |
|---------|------|-------------|
| `GET`    | `/rest/v1/items?limit=&offset=&status=`     | Paginated item list with optional status filter |
| `GET`    | `/rest/v1/items/{id}`                       | Get item details by ID |
| `POST`   | `/rest/v1/items`                            | Create a new item |
| `PATCH`  | `/rest/v1/items/{id}`                       | Partial item update |
| `DELETE` | `/rest/v1/items/{id}`                       | Delete an item |
| `GET`    | `/admin/health/liveness`                    | Liveness probe |
| `GET`    | `/admin/health/readiness`                   | Readiness probe (checks DB connection) |

#### Create Item Request Example

```json
{
  "name": "widget A",
  "description": "a quality component",
  "tags": ["electronics", "promo"]
}
```

#### Create Item Response Example

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "widget A",
  "description": "a quality component",
  "status": "active",
  "tags": ["electronics", "promo"],
  "created_at": "2024-01-15T10:30:00Z"
}
```

### 6.2 golang-app2 — Order API (`:8081`)

| Method  | Path | Description |
|---------|------|-------------|
| `GET`    | `/rest/v1/orders?limit=&offset=&status=`    | Paginated order list with optional status filter |
| `GET`    | `/rest/v1/orders/{id}`                      | Get order details by ID |
| `POST`   | `/rest/v1/orders`                           | Create a new order |
| `PATCH`  | `/rest/v1/orders/{id}`                      | Partial order update |
| `DELETE` | `/rest/v1/orders/{id}`                      | Cancel an order |
| `GET`    | `/admin/health/liveness`                    | Liveness probe |
| `GET`    | `/admin/health/readiness`                   | Readiness probe (checks DB connection) |

#### Create Order Request Example

```json
{
  "note": "expedited shipping",
  "items": [
    {
      "item_id": "550e8400-e29b-41d4-a716-446655440000",
      "quantity": 2,
      "price": 29.99
    }
  ]
}
```

#### Create Order Response Example

```json
{
  "id": "660e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "total": 59.98,
  "note": "expedited shipping",
  "items": [
    {
      "id": "...",
      "item_id": "550e8400-e29b-41d4-a716-446655440000",
      "item_name": "widget A",
      "quantity": 2,
      "price": 29.99
    }
  ],
  "created_at": "2024-01-15T10:35:00Z"
}
```

#### Order Status Transitions

```text
pending (待处理) → confirmed (已确认) → shipped (已发货)
            │
            └──────────────→ canceled (已取消)
```

---

## 7. Local Development Guide

### 7.1 Prerequisites

| Tool          | Version | Purpose |
|---------------|---------|---------|
| Go            | 1.23+   | Language runtime |
| Docker        | 24+     | Container runtime |
| Docker Compose| v2+     | Multi-container orchestration |
| Make          | any     | Build automation |
| golangci-lint | latest  | (Optional) Linter |
| Helm          | 3.1+    | (Optional) Kubernetes deployment |

### 7.2 Quick Start — Docker Compose (Recommended)

```bash
# 1. Clone and enter the repo
cd golang-system

# 2. Start the full stack (2 apps + 2 DBs + NATS + migration)
make docker-up

# 3. Run database migrations
make migrate

# 4. Verify services
curl http://localhost:8080/admin/health/readiness    # → {"status":"ok"}
curl http://localhost:8081/admin/health/readiness    # → {"status":"ok"}

# 5. Tail service logs
make docker-logs

# 6. Stop all services
make docker-down
```

#### Service Endpoints after `docker-up`

| Service                        | Address |
|--------------------------------|---------|
| golang-app  (Item service)     | http://localhost:8080 |
| golang-app2 (Order service)    | http://localhost:8081 |
| PostgreSQL (Item DB)           | localhost:5432 |
| PostgreSQL (Order DB)          | localhost:5433 |
| NATS message bus               | localhost:4222 |
| NATS monitoring dashboard      | http://localhost:8222 |

### 7.3 Quick Start — Native Go (Daily Development)

```bash
# 1. Start infrastructure only
docker compose up -d postgres-app postgres-app2 nats

# 2. Run database migrations
make migrate

# 3. Build all modules
make build

# 4. Start item service (terminal 1)
cd golang-app && ./bin/server

# 5. Start order service (terminal 2)
cd golang-app2 && ./bin/server
```

### 7.4 Go Workspace Multi-Module Development

This project uses [Go Workspaces](https://go.dev/doc/tutorial/workspaces) for multi-module development. Modules are declared in `go.work`; changes to `golang-commons` or `golang-events` take effect immediately in `golang-app` / `golang-app2` without publishing.

### 7.5 End-to-End Test Flow

```bash
# Step 1: Create an item
ITEM_ID=$(curl -s -X POST http://localhost:8080/rest/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name": "widget A", "description": "a quality component", "tags": ["demo"]}' \
  | jq -r '.id')

echo "created item: $ITEM_ID"

# Step 2: Create an order that references the item
# golang-app2 will synchronously call golang-app to validate the item exists
ORDER_ID=$(curl -s -X POST http://localhost:8081/rest/v1/orders \
  -H "Content-Type: application/json" \
  -d "{\"note\": \"first order\", \"items\": [{\"item_id\": \"$ITEM_ID\", \"quantity\": 2, \"price\": 9.99}]}" \
  | jq -r '.id')

echo "created order: $ORDER_ID"
# golang-app receives the order.created event via NATS

# Step 3: Delete the item
curl -s -X DELETE "http://localhost:8080/rest/v1/items/$ITEM_ID"

# golang-app publishes item.deleted to NATS
# golang-app2 receives item.deleted and auto-cancels the pending order

# Step 4: Verify the order has been auto-canceled
curl -s "http://localhost:8081/rest/v1/orders/$ORDER_ID" | jq '.status'
# → "canceled"
```

---

## 8. Deployment Guide

### 8.1 Building Docker Images

```bash
# Build from the golang-system root
docker build -f golang-app/Dockerfile -t golang-app:latest .
docker build -f golang-app2/Dockerfile -t golang-app2:latest .

# Tag and push to a container registry
docker tag golang-app:latest ghcr.io/<org>/golang-system/golang-app:latest
docker push ghcr.io/<org>/golang-system/golang-app:latest

docker tag golang-app2:latest ghcr.io/<org>/golang-system/golang-app2:latest
docker push ghcr.io/<org>/golang-system/golang-app2:latest
```

### 8.2 Kubernetes Deployment — Total Helm Chart

**Step 1: Modify configuration**

```bash
cd golang-system-chart
```

Edit `values.yaml` to update key per-environment settings:

- `global.imageRegistry`            — your container registry
- `golang-app.image.tag`            — target image version
- `golang-app2.image.tag`           — target image version
- `postgresql-app.auth.password`    — production DB password
- `postgresql-app2.auth.password`   — production DB password
- `golang-app2.env.ITEM_SERVICE_URL` — K8s internal service URL

**Step 2: Update Helm dependencies**

```bash
helm dependency update
```

**Step 3: Install**

```bash
helm install golang-system ./golang-system-chart \
  --namespace golang-system \
  --create-namespace
```

**Step 4: Port-forward (local access)**

```bash
kubectl -n golang-system port-forward svc/golang-system-golang-app 8080:80
kubectl -n golang-system port-forward svc/golang-system-golang-app2 8081:80

# Health checks
curl http://localhost:8080/admin/health/readiness
curl http://localhost:8081/admin/health/readiness
```

**Step 5: Uninstall**

```bash
helm uninstall golang-system -n golang-system
kubectl delete namespace golang-system
```

### 8.3 Database Migrations

Database migrations **must** run after the infrastructure is ready:

```bash
# Docker Compose local environment
make migrate

# Kubernetes environment — via Job or Init Container
# Method 1: kubectl exec
kubectl -n golang-system exec -it deploy/golang-system-golang-app -- \
  sh -c "cd /home/appuser/db && migrate -path migrations -database $DATABASE_URL up"

# Method 2: Helm Hook (add pre-install/pre-upgrade Jobs to the Chart)
```

### 8.4 Production Readiness Checklist

- [ ] Set `APP_MODE=PROD` to enable JSON structured logging
- [ ] Set strong unique passwords for all databases
- [ ] Configure resource requests and limits (CPU/memory) in Helm values
- [ ] Enable HPA (Horizontal Pod Autoscaling)
- [ ] Configure NATS JetStream persistence to guarantee no event loss
- [ ] Configure NetworkPolicy to restrict inter-service traffic
- [ ] Set up monitoring (Prometheus metrics + Grafana dashboards)
- [ ] Configure TLS for NATS and database connections
- [ ] Use external secret management (HashiCorp Vault / Kubernetes Secrets)
- [ ] Review and configure CORS for production domains

---

## 9. CI/CD Pipelines

### 9.1 CI Pipeline (`.github/workflows/ci.yml`)

Triggered on every **push** and **Pull Request** to `main` / `develop`.

```text
[Change detection] → [Lint (matrix)] → [Test (matrix)] → [Build (matrix)] → [Docker (main only)]
```

**Per-stage behavior:**

| Stage              | Behavior |
|--------------------|----------|
| **Change detection** | Uses `dorny/paths-filter` to detect which modules changed; only changed modules trigger follow-up jobs |
| **Lint**             | Runs `golangci-lint` in parallel for each changed module |
| **Test**             | Runs `go test -race -coverprofile` per changed module; uploads coverage reports |
| **Build**            | Static compile `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`; uploads binary artifacts |
| **Docker**           | Build & push multi-stage Docker image to GHCR, **only on `main`**, tags: `SHA` + `latest` |

**Smart dependency discovery:**

- **Go modules** — weekly check, per-module independent updates (`golang-commons`, `golang-events`, `golang-app`, `golang-app2`)
- **Docker** — weekly check, updates base images per service Dockerfile
- **GitHub Actions** — weekly check, updates Action versions used in workflows

### 9.2 CD Pipeline (`.github/workflows/cd.yml`)

Triggered **after CI passes** and the code is on **`main`**. Progressive three-environment deployment with approval gates.

```text
Deploy DEV  →  Deploy STAG  →  Deploy PROD
  (auto)         (approval)       (approval)
```

**Per-environment deployment steps:**

1. **Install Helm** (v3.15)
2. **Configure kubectl** (via per-environment `KUBE_CONFIG_*` Secret)
3. **helm dependency update** — refresh sub-chart dependencies
4. **helm upgrade --install** — deploy the total Chart with:
   - `--set global.environment=<env-name>`
   - `--set golang-app.image.tag=$GITHUB_SHA`
   - `--set golang-app2.image.tag=$GITHUB_SHA`
   - `--wait --timeout 10m`
5. **Smoke test** — `kubectl rollout status` to verify both services' deployment state

**Required GitHub Secrets:**

| Secret                | Environment | Purpose |
|-----------------------|-------------|---------|
| `KUBE_CONFIG_DEV`     | development | kubectl config for dev cluster |
| `KUBE_CONFIG_STAGING` | staging     | kubectl config for staging cluster |
| `KUBE_CONFIG_PROD`    | production  | kubectl config for production cluster |

**Required GitHub Environments:**

| Environment  | Configuration |
|--------------|---------------|
| `development` | No restrictions (auto-deploy) |
| `staging`     | Approval required: at least 1 team member |
| `production`  | Approval required: at least 2 team members, restricted to `main` branch |

### 9.3 Release Pipeline (`.github/workflows/release.yml`)

Triggered on pushing a **`v*` version tag** (e.g. `v1.2.0`).

```text
Build multi-arch binaries  →  Create Release  →  Publish Helm Chart
     (cross-compile)            (GitHub)           (GHCR OCI)
```

**Cross-compile matrix:**

- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64`

**Artifacts:**

- **GitHub Release** — auto-generated release notes + binaries per platform
- **Docker images** — multi-platform images pushed to GHCR
- **Helm Chart** — packaged and pushed to GHCR as an OCI artifact

**Release steps:**

```bash
# Tag and push
git tag v1.2.0
git push origin v1.2.0
```

The release pipeline automatically:

1. Cross-compiles binaries for all platforms
2. Builds and pushes multi-platform Docker images
3. Creates a GitHub Release and uploads the artifacts
4. Packages the Helm Chart and pushes it to GHCR OCI Registry

### 9.4 Dependabot Automated Dependency Updates

Automatically tracks and updates the following types of dependencies:

- **Go modules** — weekly check, per-module independent updates (`golang-commons`, `golang-events`, `golang-app`, `golang-app2`)
- **Docker** — weekly check, updates base images per service Dockerfile
- **GitHub Actions** — weekly check, updates Action versions used in workflows

---

## 10. Configuration Reference

### 10.1 golang-app Environment Variables

| Variable        | Default                          | Description |
|-----------------|----------------------------------|-------------|
| `APP_PORT`      | `8080`                           | HTTP server port |
| `APP_MODE`      | `DEV`                            | Application mode: `DEV` (colored console logs) or `PROD` (JSON structured logs) |
| `APP_NAME`      | `golang-app`                     | App name (used for log identification) |
| `LOG_LEVEL`     | `info`                           | Log level: `debug`, `info`, `warn`, `error` |
| `DB_HOST`       | `localhost`                      | PostgreSQL host |
| `DB_PORT`       | `5432`                           | PostgreSQL port |
| `DB_USER`       | `app`                            | PostgreSQL user |
| `DB_NAME`       | `golang-app`                     | PostgreSQL database name |
| `DB_PASSWORD`   | ``                               | PostgreSQL password |
| `DB_SSLMODE`    | `disable`                        | PostgreSQL SSL mode |
| `NATS_URL`      | `nats://localhost:4222`          | NATS server URL |
| `QUEUE_GROUP`   | `golang-app-queue`               | NATS queue group name (for load balancing) |

### 10.2 golang-app2 Environment Variables

In addition to the common variables above, includes:

| Variable          | Default                       | Description |
|-------------------|-------------------------------|-------------|
| `APP_PORT`        | `8081`                        | HTTP server port |
| `APP_NAME`        | `golang-app2`                 | App name (used for log identification) |
| `DB_NAME`         | `golang-app2`                 | PostgreSQL database name |
| `ITEM_SERVICE_URL`| `http://localhost:8080`       | Base URL of golang-app (Item service) |
| `QUEUE_GROUP`     | `golang-app2-queue`           | NATS queue group name |

---

## 11. Command Reference

| Command                | Description |
|------------------------|-------------|
| `make all`             | Lint + test + build |
| `make build`           | Build all service binaries to `<service>/bin/server` |
| `make test`            | Run tests with race detection for all modules |
| `make lint`            | Run `golangci-lint` on all modules |
| `make tidy`            | Run `go mod tidy` on all modules |
| `make docker-up`       | Start the full system (Docker Compose) |
| `make docker-down`     | Stop and remove all containers and volumes |
| `make docker-logs`     | Tail logs of the two application services |
| `make migrate`         | Run all services' database migrations |
| `make clean`           | Remove build artifacts (`bin/`, `.tmp/`) |
| `make generate`        | Run `go generate` in all services |
| `make helm-deps`       | Update Helm Chart dependencies |
| `make helm-template`   | Render Helm templates locally |
| `make helm-install`    | Deploy to Kubernetes |
| `make helm-uninstall`  | Uninstall from Kubernetes |
