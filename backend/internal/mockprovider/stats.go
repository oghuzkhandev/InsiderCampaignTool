package mockprovider

import (
	"slices"
	"sync"
	"time"
)

// Stats, sağlayıcının BELLEKTEKİ istatistikleridir (ADR-025): yeniden başlatınca
// sıfırlanır; yalnızca kabul kayıtları PostgreSQL'de kalıcıdır.
type Stats struct {
	mu sync.Mutex

	attempts map[string][]time.Time // kanal → her isteğin varış zamanı (retry'lar dahil)

	totalRequests     int
	uniqueAccepts     int
	duplicateRequests int
	responsesDropped  int
	rateLimited       int
	serverErrors      int
	keyReuseRejected  int
	invalidRequests   int

	deliveries map[string]int // anahtar → bu süreçte kaç kez İLK kez kabul edildi
}

// NewStats, boş istatistikler oluşturur.
func NewStats() *Stats {
	return &Stats{attempts: map[string][]time.Time{}, deliveries: map[string]int{}}
}

func (s *Stats) attempt(channel string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRequests++
	s.attempts[channel] = append(s.attempts[channel], at)
}

func (s *Stats) accepted(key string, created bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if created {
		s.uniqueAccepts++
		s.deliveries[key]++
		return
	}
	s.duplicateRequests++
}

func (s *Stats) dropped()         { s.add(&s.responsesDropped) }
func (s *Stats) limited()         { s.add(&s.rateLimited) }
func (s *Stats) failed()          { s.add(&s.serverErrors) }
func (s *Stats) keyReused()       { s.add(&s.keyReuseRejected) }
func (s *Stats) invalid()         { s.add(&s.invalidRequests) }
func (s *Stats) add(counter *int) { s.mu.Lock(); *counter++; s.mu.Unlock() }

// Snapshot, istatistiklerin o anki kopyasıdır.
type Snapshot struct {
	TotalRequests int `json:"total_requests"`
	// UniqueAccepts, ilk kez kaydedilen kabullerdir.
	UniqueAccepts int `json:"unique_accepts"`
	// DuplicateRequests, kayıtlı sonucu alan tekrar isteklerdir.
	DuplicateRequests int `json:"duplicate_requests"`
	// DuplicateDeliveries, aynı mesajın ikinci teslimidir (beklenen 0). Benzersiz
	// anahtar yüzünden yapısal olarak 0'dır; kanıt değeri, tekrar isteklerinin
	// buna rağmen sayılmasıdır (DuplicateRequests).
	DuplicateDeliveries int `json:"duplicate_deliveries"`
	ResponsesDropped    int `json:"responses_dropped"`
	RateLimited         int `json:"rate_limited"`
	ServerErrors        int `json:"server_errors"`
	KeyReuseRejected    int `json:"key_reuse_rejected"`
	InvalidRequests     int `json:"invalid_requests"`
	// Attempts, kanal bazında her isteğin varış zamanıdır (retry'lar dahil).
	Attempts map[string][]time.Time `json:"attempts"`
}

// Snapshot, o anki istatistiklerin bağımsız bir kopyasını döndürür.
func (s *Stats) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{
		TotalRequests:     s.totalRequests,
		UniqueAccepts:     s.uniqueAccepts,
		DuplicateRequests: s.duplicateRequests,
		ResponsesDropped:  s.responsesDropped,
		RateLimited:       s.rateLimited,
		ServerErrors:      s.serverErrors,
		KeyReuseRejected:  s.keyReuseRejected,
		InvalidRequests:   s.invalidRequests,
		Attempts:          make(map[string][]time.Time, len(s.attempts)),
	}
	for _, n := range s.deliveries {
		if n > 1 {
			snap.DuplicateDeliveries += n - 1
		}
	}
	for ch, ts := range s.attempts {
		snap.Attempts[ch] = slices.Clone(ts)
	}
	return snap
}

// MaxInWindow, zaman damgalarının herhangi bir kayan pencerede (w uzunluğunda,
// yarı açık [t, t+w)) en çok kaç tane olduğunu döndürür. Pencere her
// zaman damgasında başlayabilir. Girdinin sıralı olması gerekmez.
func MaxInWindow(ts []time.Time, w time.Duration) int {
	sorted := slices.Clone(ts)
	slices.SortFunc(sorted, func(a, b time.Time) int { return a.Compare(b) })
	best, j := 0, 0
	for i := range sorted {
		if j < i {
			j = i
		}
		for j < len(sorted) && sorted[j].Sub(sorted[i]) < w {
			j++
		}
		best = max(best, j-i)
	}
	return best
}
