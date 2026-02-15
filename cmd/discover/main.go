package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discover/internal/auth"
	"discover/internal/config"
	"discover/internal/db"
	"discover/internal/ingest"
	"discover/internal/scheduler"
	"discover/internal/server"
	"discover/internal/store"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "path to config file")
	flag.Parse()

	cfg, created, err := config.LoadOrInit(configPath)
	if err != nil {
		log.Fatal(err)
	}
	if created {
		fmt.Printf("Created default config at %s. Edit it (especially user_secret, admin_secret, and TLS paths), then rerun.\n", configPath)
		os.Exit(0)
	}

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()
	st := store.New(database)

	guard, err := auth.New(cfg.AdminSecret, cfg.AdminBindCIDRs)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}
	userGuard, err := auth.NewUserGuard(cfg.UserName, cfg.UserSecret)
	if err != nil {
		log.Fatalf("init user auth: %v", err)
	}
	ingester := ingest.New(cfg, st)
	sched := scheduler.New(cfg.DailyIngestTime, ingester)

	api := server.New(cfg, st, sched, ingester, guard, userGuard, server.AssetsHandler())
	httpServer := &http.Server{
		Addr:         cfg.ListenAddress,
		Handler:      api.Routes(),
		ReadTimeout:  time.Duration(cfg.HTTPReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTPWriteTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(cfg.HTTPIdleTimeoutSec) * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	sched.Start(ctx)

	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shCtx)
	}()

	log.Printf("starting discover on %s (tls=%v)", cfg.ListenAddress, cfg.EnableTLS)
	if cfg.EnableTLS {
		err = httpServer.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath)
	} else {
		err = httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
