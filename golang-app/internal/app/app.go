// Package app is the composition root: it wires dependencies and starts the server.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMW "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/database"
	"github.com/darrell/golang-system/golang-commons/httputil"
	"github.com/darrell/golang-system/golang-events"

	"github.com/darrell/golang-system/golang-app/internal/adapter/messaging"
	"github.com/darrell/golang-system/golang-app/internal/adapter/repository"
	"github.com/darrell/golang-system/golang-app/internal/config"
	itemHandler "github.com/darrell/golang-system/golang-app/internal/ports/http/event"
	"github.com/darrell/golang-system/golang-app/internal/service"
)

// App is the running application.
type App struct {
	ctx context.Context
	cfg *config.Config
	log zerolog.Logger
	srv *http.Server
	db  *pgxpool.Pool
	nc  *nats.Conn
}

// New constructs the App (does not start anything).
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	log := *zerolog.DefaultContextLogger
	return &App{ctx: ctx, cfg: cfg, log: log}, nil
}

// Start initializes connections and begins serving HTTP.
func (a *App) Start() error {
	// DB
	dsn := database.Config{
		Host: a.cfg.DBHost, Port: a.cfg.DBPort, User: a.cfg.DBUser,
		Name: a.cfg.DBName, Password: a.cfg.DBPassword, SSLMode: a.cfg.DBSSLMode,
	}.DSN()
	pool, err := pgxpool.New(a.ctx, dsn)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	a.db = pool
	if err := pool.Ping(a.ctx); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}

	// NATS
	nc, err := nats.Connect(a.cfg.NATSURL, nats.Timeout(5*time.Second))
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}
	a.nc = nc

	// Adapters
	itemRepo := repository.NewItemPostgres(pool)
	itemPub := messaging.NewNatsPublisher(nc, a.log)
	itemSvc := service.NewItemService(itemRepo, itemPub, a.log)

	// Subscribe to events
	a.subscribeNATS(nc, itemSvc, a.cfg.QueueGroup)

	// HTTP router
	r := chi.NewRouter()
	r.Use(chiMW.RealIP)
	r.Use(httputil.RequestIDMiddleware)
	r.Use(httputil.AuthContextMiddleware)
	r.Use(httputil.RecoveryMiddleware(a.log))
	r.Use(httputil.MaxBodySizeMiddleware(1 << 20))
	r.Use(httputil.ContentTypeMiddleware)
	r.Use(httputil.LoggingMiddleware(a.log))

	// Health endpoints
	checker := database.NewHealthChecker(pool)
	r.Get("/admin/health/liveness", func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/admin/health/readiness", func(w http.ResponseWriter, r *http.Request) {
		if err := checker(r.Context()); err != nil {
			httputil.Error(w, err)
			return
		}
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// REST API
	itemHandler.NewItemHandler(itemSvc, a.log).Routes(r)

	addr := fmt.Sprintf(":%d", a.cfg.AppPort)
	a.srv = &http.Server{Addr: addr, Handler: r}

	a.log.Info().Int("port", a.cfg.AppPort).Msg("server starting")
	if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server and drains connections.
func (a *App) Shutdown(ctx context.Context) error {
	if a.srv != nil {
		if err := a.srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
	}
	if a.db != nil {
		a.db.Close()
	}
	if a.nc != nil {
		a.nc.Drain()
		a.nc.Close()
	}
	return nil
}

// subscribeNATS wires the NATS subscribers with queue-group load balancing.
func (a *App) subscribeNATS(nc *nats.Conn, svc service.ItemService, qg string) {
	// Subscribe to order.created — record item references.
	if _, err := nc.QueueSubscribe(events.SubjectOrderCreated, qg, func(m *nats.Msg) {
		var evt events.Event[events.OrderCreatedData]
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			a.log.Error().Err(err).Msg("decode order.created")
			return
		}
		if err := svc.HandleOrderCreated(context.Background(), &evt); err != nil {
			a.log.Error().Err(err).Msg("handle order.created")
		}
	}); err != nil {
		a.log.Error().Err(err).Msg("subscribe order.created")
	}

	// Request-Reply: item.get_by_id
	if _, err := nc.QueueSubscribe(events.SubjectItemGetByID, qg, func(m *nats.Msg) {
		var req events.GetItemRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			a.log.Error().Err(err).Msg("decode item.get_by_id")
			return
		}
		resp, err := svc.HandleGetItemByID(context.Background(), &req)
		if err != nil {
			a.log.Error().Err(err).Msg("handle item.get_by_id")
			_ = m.Respond([]byte(`{"error":"not found"}`))
			return
		}
		data, _ := json.Marshal(resp)
		_ = m.Respond(data)
	}); err != nil {
		a.log.Error().Err(err).Msg("subscribe item.get_by_id")
	}

	// External item creation events
	if _, err := nc.QueueSubscribe(events.SubjectItemExternalCreate, qg, func(m *nats.Msg) {
		var evt events.Event[events.ItemCreatedData]
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			a.log.Error().Err(err).Msg("decode item.external.create")
			return
		}
		if err := svc.HandleExternalCreate(context.Background(), &evt); err != nil {
			a.log.Error().Err(err).Msg("handle item.external.create")
		}
	}); err != nil {
		a.log.Error().Err(err).Msg("subscribe item.external.create")
	}

	a.log.Info().Str("queue_group", qg).Msg("nats subscriptions registered")
}
