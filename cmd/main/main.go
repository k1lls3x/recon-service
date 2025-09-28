package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
"runtime"
			_ "net/http/pprof"
	"recon-service/internal/config"
	serverhttp "recon-service/server/http"
)

func main() {
		if runtime.GOMAXPROCS(0) < runtime.NumCPU() {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	
	cfg := config.Load()
	logger := config.SetupLogger(cfg)

	r := serverhttp.NewRouter(cfg, logger)

	srv := &http.Server{Addr: cfg.Addr(), Handler: r}
	logger.Info().Str("addr", cfg.Addr()).Msg("server starting")

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("listen")
		}
	}()

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("server shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	logger.Info().Msg("bye")
}
