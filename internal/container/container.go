package container

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"

	"github.com/saifullinartem/saifullin_golang/internal/config"
	transport "github.com/saifullinartem/saifullin_golang/internal/http"
	"github.com/saifullinartem/saifullin_golang/internal/http/middleware"
	httpcreate "github.com/saifullinartem/saifullin_golang/internal/http/trip/create"
	httpfinish "github.com/saifullinartem/saifullin_golang/internal/http/trip/finish"
	httpget "github.com/saifullinartem/saifullin_golang/internal/http/trip/get"
	"github.com/saifullinartem/saifullin_golang/internal/infra/postgres"
	"github.com/saifullinartem/saifullin_golang/internal/infra/postgres/transaction"
	"github.com/saifullinartem/saifullin_golang/internal/infra/telemetry"
	repository "github.com/saifullinartem/saifullin_golang/internal/repository/trip"
	"github.com/saifullinartem/saifullin_golang/internal/usecase/trip/create"
	"github.com/saifullinartem/saifullin_golang/internal/usecase/trip/finish"
	"github.com/saifullinartem/saifullin_golang/internal/usecase/trip/get"
)

type Container struct {
	Server    *http.Server
	Logger    *slog.Logger
	pool      *pgxpool.Pool
	telemetry *telemetry.Providers
}

func New(ctx context.Context, cfg config.Config) (result *Container, err error) {
	logger := telemetry.Logger(os.Stdout, cfg.LogLevel, cfg.OTelServiceName)
	tel, err := telemetry.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	c := &Container{Logger: logger, telemetry: tel}
	defer func() {
		if err != nil {
			c.Close(ctx)
		}
	}()
	pc, err := postgres.Config(cfg)
	if err != nil {
		return nil, err
	}
	tracer, err := postgres.NewTracer(tel.Traces, tel.Metrics)
	if err != nil {
		return nil, err
	}
	pc.ConnConfig.Tracer = tracer
	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	c.pool, err = pgxpool.NewWithConfig(connectCtx, pc)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err = c.pool.Ping(connectCtx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	repo := repository.NewRepository(c.pool, cfg.DatabaseQueryTimeout)
	history := repository.NewHistoryRepository(c.pool, cfg.DatabaseQueryTimeout)
	tx := transaction.NewTxManager(c.pool, cfg.DatabaseQueryTimeout)
	meter := tel.Metrics.Meter("tripgo/business")
	events, err := telemetry.NewEvents(meter)
	if err != nil {
		return nil, err
	}
	_, err = meter.Int64ObservableGauge("tripgo.trips.active", metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		queryCtx, cancelQuery := context.WithTimeout(ctx, cfg.DatabaseQueryTimeout)
		defer cancelQuery()
		count, queryErr := repo.CountActive(queryCtx)
		if queryErr != nil {
			return fmt.Errorf("count active trips: %w", queryErr)
		}
		o.Observe(count)
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("active trips gauge: %w", err)
	}
	clock := telemetry.Clock{}
	handler := transport.New(httpcreate.New(create.New(repo, history, tx, clock, events, logger)), httpget.New(get.New(repo)), httpfinish.New(finish.New(repo, history, tx, clock, events, logger)), c.pool, cfg.DatabaseQueryTimeout)
	router := transport.NewRouter(handler)
	mw, err := middleware.New(logger, tel.Traces, tel.Metrics, router)
	if err != nil {
		return nil, err
	}
	c.Server = &http.Server{Addr: cfg.HTTPAddr, Handler: mw.Wrap(router), ReadTimeout: cfg.HTTPReadTimeout, ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout, WriteTimeout: cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout}
	return c, nil
}

func (c *Container) Close(ctx context.Context) {
	if err := c.telemetry.Shutdown(ctx); err != nil {
		c.Logger.WarnContext(ctx, "telemetry shutdown incomplete", "error", err)
	}
	if c.pool != nil {
		c.pool.Close()
	}
}

func LogError(ctx context.Context, err error) {
	telemetry.Logger(os.Stdout, "error", "trip-service").ErrorContext(ctx, "trip service stopped", "error", err)
}
