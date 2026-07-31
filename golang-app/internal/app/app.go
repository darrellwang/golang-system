// Package app is the composition root: it wires dependencies and starts the server.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/database"
	"github.com/darrell/golang-system/golang-commons/httputil"
	commonsmsg "github.com/darrell/golang-system/golang-commons/messaging"
	"github.com/darrell/golang-system/golang-commons/outbox"
	"github.com/darrell/golang-system/golang-events"

	"github.com/darrell/golang-system/golang-app/internal/adapter/messaging"
	"github.com/darrell/golang-system/golang-app/internal/adapter/repository"
	"github.com/darrell/golang-system/golang-app/internal/config"
	itemHandler "github.com/darrell/golang-system/golang-app/internal/ports/http/event"
	"github.com/darrell/golang-system/golang-app/internal/service"
)

// App is the running application.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config
	log    zerolog.Logger
	srv    *http.Server
	db     *pgxpool.Pool
	nc     *nats.Conn
	js     nats.JetStreamContext
}

// New constructs the App (does not start anything).
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	log := *zerolog.DefaultContextLogger
	ctx, cancel := context.WithCancel(ctx)
	return &App{ctx: ctx, cancel: cancel, cfg: cfg, log: log}, nil
}

// Start initializes connections and begins serving HTTP.
func (a *App) Start() error {
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

	nc, js, err := commonsmsg.ConnectJetStream(a.cfg.NATSURL, nil)
	if err != nil {
		return fmt.Errorf("nats jetstream: %w", err)
	}
	a.nc, a.js = nc, js

	itemRepo := repository.NewItemPostgres(pool)
	outboxStore := outbox.NewStore(pool)
	itemPub := messaging.NewOutboxPublisher(outboxStore, a.log)
	tx := &database.Transactor{Pool: pool}
	itemSvc := service.NewItemService(itemRepo, itemPub, tx, a.log)

	go outbox.Relay(a.ctx, outboxStore, js, a.log, 500*time.Millisecond)
	a.subscribeNATS(nc, js, itemSvc, a.cfg.QueueGroup)

	r := chi.NewRouter()
	if len(a.cfg.CORSAllowedOrigins) > 0 {
		r.Use(httputil.CORSMiddleware(&httputil.CORSConfig{
			AllowedOrigins:   a.cfg.CORSAllowedOrigins,
			AllowedMethods:   a.cfg.CORSAllowedMethods,
			AllowedHeaders:   a.cfg.CORSAllowedHeaders,
			AllowCredentials: a.cfg.CORSAllowCredentials,
		}))
	}
	r.Use(httputil.RequestIDMiddleware)
	r.Use(httputil.AuthContextMiddleware(&httputil.AuthConfig{TrustHeaders: a.cfg.AuthTrustHeaders}))
	r.Use(httputil.RecoveryMiddleware(a.log))
	r.Use(httputil.MaxBodySizeMiddleware(1 << 20))
	r.Use(httputil.ContentTypeMiddleware)
	r.Use(httputil.MetricsMiddleware)
	r.Use(httputil.LoggingMiddleware(a.log))

	r.Handle("/metrics", httputil.MetricsHandler())

	checker := database.NewHealthChecker(pool)
	r.Get("/admin/health/liveness", func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/admin/health/readiness", func(w http.ResponseWriter, r *http.Request) {
		if err := checker(r.Context()); err != nil {
			httputil.Error(w, err)
			return
		}
		if !a.nc.IsConnected() {
			httputil.Error(w, fmt.Errorf("nats disconnected"))
			return
		}
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	r.Group(func(r chi.Router) {
		if a.cfg.AuthRequireJWT {
			r.Use(httputil.RequireAuthenticated)
		}
		itemHandler.NewItemHandler(itemSvc, a.log).Routes(r)
	})

	addr := fmt.Sprintf(":%d", a.cfg.AppPort)
	a.srv = &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	a.log.Info().Int("port", a.cfg.AppPort).Bool("tls", a.cfg.TLSEnabled).Msg("server starting")

	if a.cfg.TLSEnabled && a.cfg.TLSCertFile != "" && a.cfg.TLSKeyFile != "" {
		err = a.srv.ListenAndServeTLS(a.cfg.TLSCertFile, a.cfg.TLSKeyFile)
	} else {
		err = a.srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server and drains connections.
func (a *App) Shutdown(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.srv != nil {
		if err := a.srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
	}
	if a.db != nil {
		a.db.Close()
	}
	if a.nc != nil {
		_ = a.nc.Drain()
		a.nc.Close()
	}
	return nil
}

func (a *App) subscribeNATS(nc *nats.Conn, js nats.JetStreamContext, svc service.ItemService, qg string) {
	if _, err := commonsmsg.QueueSubscribeDurable(js, events.SubjectOrderCreated, qg+"-order-created", func(ctx context.Context, _ string, data []byte) error {
		var evt events.Event[events.OrderCreatedData]
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		ok, err := outbox.TryMarkProcessed(ctx, a.db, evt.ID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return svc.HandleOrderCreated(ctx, &evt)
	}, a.log); err != nil {
		a.log.Error().Err(err).Msg("subscribe order.created")
	}

	// Request-reply stays on Core NATS.
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

	if _, err := commonsmsg.QueueSubscribeDurable(js, events.SubjectItemExternalCreate, qg+"-item-ext", func(ctx context.Context, _ string, data []byte) error {
		var evt events.Event[events.ItemCreatedData]
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		ok, err := outbox.TryMarkProcessed(ctx, a.db, evt.ID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return svc.HandleExternalCreate(ctx, &evt)
	}, a.log); err != nil {
		a.log.Error().Err(err).Msg("subscribe item.external.create")
	}

	a.log.Info().Str("queue_group", qg).Msg("nats/jetstream subscriptions registered")
}
