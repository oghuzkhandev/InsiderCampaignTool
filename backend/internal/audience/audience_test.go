package audience

import "testing"

func TestHasRecipients(t *testing.T) {
	tests := []struct {
		name string
		c    Counts
		want bool
	}{
		{"hiç eşleşme yok", Counts{}, false},
		{"eşleşme var ama izinli alıcı yok", Counts{Total: 10}, false},
		{"yalnızca SMS", Counts{Total: 10, SMS: 1}, true},
		{"yalnızca e-posta", Counts{Total: 10, Email: 2}, true},
		{"iki kanal", Counts{Total: 10, SMS: 3, Email: 4}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.HasRecipients(); got != tt.want {
				t.Fatalf("HasRecipients = %v, beklenen %v", got, tt.want)
			}
		})
	}
}
