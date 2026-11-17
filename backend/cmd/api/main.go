package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("api başlıyor", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("api durdu", "error", err)
		os.Exit(1)
	}
}
