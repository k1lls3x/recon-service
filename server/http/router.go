package serverhttp

import (
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"recon-service/internal/config"
	"recon-service/internal/middleware"
	"recon-service/server/http/handlers"
	recHnd "recon-service/internal/reconcile/handler"
)

func NewRouter(cfg config.Config, logger zerolog.Logger) *chi.Mux {
	r := chi.NewRouter()

	// порядок важен: recover -> requestID -> logging -> cors -> limit
	r.Use(middleware.Recover(logger))
	r.Use(middleware.RequestID())
	r.Use(middleware.Logging(logger))
	r.Use(middleware.CORS(cfg.AllowOrigins))
	r.Use(middleware.LimitBytes(int64(cfg.MaxUploadMB) * 1024 * 1024))

	// health-check
	r.Get("/health", handlers.Health)

	// основной эндпоинт
	r.Post("/reconcile", recHnd.Reconcile(cfg, logger))

	return r
}
