package httpapi

import (
	"encoding/json"
	"net/http"
)

// reason, bir hatanın sebebini taşır. Kullanıcıya gösterilecek Türkçe
// açıklama arayüzde sebep koduna göre üretilir; model çıktısı ham olarak
// iletilmez.
type reason struct {
	Code  string `json:"code"`
	Field string `json:"field,omitempty"`
	Text  string `json:"text,omitempty"`
	// Count, sebebin kaç alıcıyı etkilediğidir (ör. render sınırı aşımı).
	Count int `json:"count,omitempty"`
}

type errorDetail struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Reasons []reason `json:"reasons,omitempty"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

// writeJSON, gövdeyi önce serileştirir; serileştirme başarısızsa 500 döner,
// böylece yarım yanıt gitmez.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(`{"error":{"code":"INTERNAL","message":"Beklenmeyen bir hata oluştu."}}`)
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, code, message string, reasons ...reason) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Reasons: reasons}})
}
