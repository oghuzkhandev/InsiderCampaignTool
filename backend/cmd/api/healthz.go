package main

import "net/http"

// healthz API süreci ayaktaysa 200 döner. Veritabanı kontrolü yapmaz.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
