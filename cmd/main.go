package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/YashGholap/ProxyRack-Manager/internal/config"
	"github.com/YashGholap/ProxyRack-Manager/internal/handlers"
	"github.com/YashGholap/ProxyRack-Manager/internal/pool"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	log.Printf("Starting proxyrack-pool-manager")
	log.Printf("  Max threads:      %d", cfg.MaxThreads)
	log.Printf("  Max queue depth:  %d", cfg.MaxQueueDepth)
	log.Printf("  Request timeout:  %s", cfg.RequestTimeout)
	log.Printf("  Proxyrack URL:    %s", cfg.ProxyrackBaseURL)
	log.Printf("  Port:             %d", cfg.Port)

	pm := pool.NewManager(cfg.MaxThreads, cfg.MaxQueueDepth)

	h := handlers.New(pm, cfg)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received %s, shutting down...", sig)
		server.Close()
	}()

	log.Printf("Listening on %s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}

	log.Println("Server stopped.")
}
