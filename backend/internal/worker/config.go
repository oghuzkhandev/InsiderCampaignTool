package worker

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Ortam değişkenleri. Tanımsız olanlar DefaultConfig değerinde kalır. Testlerde
// süreler kısaltılabilir (SND-001); uygulamanın varsayılanları değişmez.
const (
	EnvCapacity        = "WORKER_CAPACITY"
	EnvMaxAttempts     = "WORKER_MAX_ATTEMPTS"
	EnvLease           = "WORKER_LEASE"
	EnvReapInterval    = "WORKER_REAP_INTERVAL"
	EnvPollInterval    = "WORKER_POLL_INTERVAL"
	EnvShutdownTimeout = "WORKER_SHUTDOWN_TIMEOUT"
	EnvRate            = "WORKER_RATE"
	EnvSendBurst       = "WORKER_SEND_BURST"
	EnvBackoffBase     = "WORKER_BACKOFF_BASE"
	EnvBackoffCap      = "WORKER_BACKOFF_CAP"
	EnvDBBackoffMin    = "WORKER_DB_BACKOFF_MIN"
	EnvDBBackoffMax    = "WORKER_DB_BACKOFF_MAX"
)

// ConfigFromEnv, DefaultConfig'i ortam değişkenleriyle günceller ve doğrular.
func ConfigFromEnv() (Config, error) {
	c := DefaultConfig()
	for _, d := range []struct {
		name string
		dst  *time.Duration
	}{
		{EnvLease, &c.Lease},
		{EnvReapInterval, &c.ReapInterval},
		{EnvPollInterval, &c.PollInterval},
		{EnvShutdownTimeout, &c.ShutdownTimeout},
		{EnvBackoffBase, &c.BackoffBase},
		{EnvBackoffCap, &c.BackoffCap},
		{EnvDBBackoffMin, &c.DBBackoffMin},
		{EnvDBBackoffMax, &c.DBBackoffMax},
	} {
		if v := os.Getenv(d.name); v != "" {
			dur, err := time.ParseDuration(v)
			if err != nil {
				return Config{}, fmt.Errorf("worker: %s geçersiz süre %q: %w", d.name, v, err)
			}
			*d.dst = dur
		}
	}
	for _, i := range []struct {
		name string
		dst  *int
	}{
		{EnvCapacity, &c.Capacity},
		{EnvMaxAttempts, &c.MaxAttempts},
		{EnvSendBurst, &c.SendBurst},
	} {
		if v := os.Getenv(i.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return Config{}, fmt.Errorf("worker: %s geçersiz sayı %q: %w", i.name, v, err)
			}
			*i.dst = n
		}
	}
	if v := os.Getenv(EnvRate); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, fmt.Errorf("worker: %s geçersiz sayı %q: %w", EnvRate, v, err)
		}
		c.Rate = f
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
