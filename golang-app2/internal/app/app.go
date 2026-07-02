package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMW "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/database"
	"github.com/darrell/golang-system/golang-commons/httputil"
	commonsmsg "github.com/darrell/golang-system/golang-commons/messaging"
	"github.com/darrell/golang-system/golang-events"

	"github.com/darrell/golang-system/golang-app2/internal/adapter/messaging"
	"github.com/darrell/golang-system/golang-app2/internal/adapter/repository"
	"github.com/darrell/golang-system/golang-app2/internal/client"
	"github.com/darrell/golang-system/golang-app2/internal/config"
	orderHandler "github.com/darrell/golang-system/golang-app2/internal/ports/http/event"
	"github.com/darrell/golang-system/golang-app2/internal/service"
)

type App struct {
	ctx context.Context
	cfg *config.Config
	log zerolog.Logger
	srv *http.Server
	db  *pgxpool.Pool
	nc  *nats.Conn
}

func New(ctx context.Context, cfg *config.Config) (*App, error) {
	log := *zerolog.DefaultContextLogger
	return &App{ctx: ctx, cfg: cfg, log: log}, nil
}

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

	nc, err := commonsmsg.Connect(a.cfg.NATSURL)
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}
	a.nc = nc

	orderRepo := repository.NewOrderPostgres(pool)
	orderPub := messaging.NewNatsPublisher(nc, a.log)
	itemClient := client.NewItemClient(a.cfg.ItemServiceURL)
	orderSvc := service.NewOrderService(orderRepo, orderPub, itemClient, a.log)

	a.subscribeNATS(nc, orderSvc, a.cfg.QueueGroup)

	r := chi.NewRouter()

	// CORS middleware (must be first)
	if len(a.cfg.CORSAllowedOrigins) > 0 {
		corsCfg := &httputil.CORSConfig{
			AllowedOrigins:   a.cfg.CORSAllowedOrigins,
			AllowedMethods:   a.cfg.CORSAllowedMethods,
			AllowedHeaders:   a.cfg.CORSAllowedHeaders,
			AllowCredentials: a.cfg.CORSAllowCredentials,
		}
		r.Use(httputil.CORSMiddleware(corsCfg))
	}

	r.Use(chiMW.RealIP)
	r.Use(httputil.RequestIDMiddleware)
	r.Use(httputil.AuthContextMiddleware)
	r.Use(httputil.RecoveryMiddleware(a.log))
	r.Use(httputil.MaxBodySizeMiddleware(1 << 20))
	r.Use(httputil.ContentTypeMiddleware)
	r.Use(httputil.MetricsMiddleware)
	r.Use(httputil.LoggingMiddleware(a.log))

	// Prometheus metrics endpoint
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
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	orderHandler.NewOrderHandler(orderSvc, a.log).Routes(r)

	addr := fmt.Sprintf(":%d", a.cfg.AppPort)
	a.srv = &http.Server{Addr: addr, Handler: r}

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

func (a *App) subscribeNATS(nc *nats.Conn, svc service.OrderService, qg string) {
	if _, err := nc.QueueSubscribe(events.SubjectItemDeleted, qg, func(m *nats.Msg) {
		var evt events.Event[events.ItemDeletedData]
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			a.log.Error().Err(err).Msg("decode item.deleted")
			return
		}
		if err := svc.HandleItemDeleted(context.Background(), &evt); err != nil {
			a.log.Error().Err(err).Msg("handle item.deleted")
		}
	}); err != nil {
		a.log.Error().Err(err).Msg("subscribe item.deleted")
	}
	if _, err := nc.QueueSubscribe(events.SubjectItemUpdated, qg, func(m *nats.Msg) {
		var evt events.Event[events.ItemUpdatedData]
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			a.log.Error().Err(err).Msg("decode item.updated")
			return
		}
		if err := svc.HandleItemUpdated(context.Background(), &evt); err != nil {
			a.log.Error().Err(err).Msg("handle item.updated")
		}
	}); err != nil {
		a.log.Error().Err(err).Msg("subscribe item.updated")
	}
	a.log.Info().Str("queue_group", qg).Msg("nats subscriptions registered")
}
