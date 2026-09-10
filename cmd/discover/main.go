package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"discover/internal/auth"
	"discover/internal/buildinfo"
	"discover/internal/config"
	"discover/internal/db"
	"discover/internal/ingest"
	"discover/internal/scheduler"
	"discover/internal/server"
	"discover/internal/store"
)

func main() {
	var configPath string
	var showVersion, checkConfig, databasePathOnly bool
	flag.StringVar(&configPath, "config", "config.json", "path to config file")
	flag.BoolVar(&showVersion, "version", false, "print build information and exit")
	flag.BoolVar(&checkConfig, "check-config", false, "validate existing configuration without modifying files")
	flag.BoolVar(&databasePathOnly, "database-path", false, "print configured database path without opening it")
	flag.Parse()
	if showVersion {
		fmt.Println(buildinfo.String())
		return
	}
	if checkConfig || databasePathOnly {
		cfg, err := config.Load(configPath)
		if err != nil {
			log.Fatal(err)
		}
		if databasePathOnly {
			fmt.Println(cfg.DatabasePath)
			return
		}
		if cfg.EnableTLS {
			if _, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil {
				log.Fatalf("TLS certificate/key: %v", err)
			}
		}
		fmt.Println("Configuration is valid; database and network were not accessed.")
		return
	}

	cfg, created, err := config.LoadOrInit(configPath)
	if err != nil {
		log.Fatal(err)
	}
	if created {
		fmt.Printf("Created default config at %s. Edit it (especially user_secret, admin_secret, and TLS paths), then rerun.\n", configPath)
		os.Exit(0)
	}
	if missing, err := config.MissingKeys(configPath); err == nil && len(missing) > 0 {
		log.Printf("config: warning: missing key(s) in %s: %s", configPath, strings.Join(missing, ", "))
		log.Printf("config: warning: existing values are not overwritten; consider adding missing keys explicitly")
	}

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()
	st := store.New(database)
	if err := st.Prepare(context.Background(), cfg.DedupeTitleKeyChars); err != nil {
		log.Fatalf("prepare article data: %v", err)
	}

	guard, err := auth.New(cfg.AdminSecret, cfg.AdminBindCIDRs)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}
	userGuard, err := auth.NewUserGuard(cfg.UserName, cfg.UserSecret)
	if err != nil {
		log.Fatalf("init user auth: %v", err)
	}
	ingester := ingest.New(cfg, st)
	sched := scheduler.New(cfg.DailyIngestTime, cfg.IngestIntervalMinutes, ingester)

	api := server.New(cfg, st, sched, ingester, guard, userGuard, server.AssetsHandler())
	defer api.Shutdown()
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
	defer sched.Shutdown()

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shCtx)
	}()

	log.Printf("starting discover %s on %s (tls=%v)", buildinfo.String(), cfg.ListenAddress, cfg.EnableTLS)
	if cfg.EnableTLS {
		err = httpServer.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath)
	} else {
		err = httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP server stopped: %v", err)
	}
	stop()
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		sched.Shutdown()
		api.Shutdown()
		_ = database.Close()
		os.Exit(1)
	}
}
