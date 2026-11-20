package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/seed"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/testdb"
)

// Arayüzün boş ekranda önerdiği örnek brief'ler (frontend/src/exampleBriefs.json, tek
// kaynak). Kullanıcıya "bunlar desteklenir" denildiği için her örnek gerçekten
// desteklenmeli: geçerli bir segment üretmeli ve demo verisinde alıcı bulmalı.
type exampleBrief struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Brief string `json:"brief"`
}

func loadExampleBriefs(t *testing.T) []exampleBrief {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "frontend", "src", "exampleBriefs.json"))
	if err != nil {
		t.Fatalf("örnek brief dosyası okunamadı: %v", err)
	}
	var list []exampleBrief
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("örnek brief dosyası geçersiz: %v", err)
	}
	return list
}

func TestExampleBriefsAreSupportedAndFindRecipients(t *testing.T) {
	examples := loadExampleBriefs(t)
	if n := len(examples); n < 2 || n > 3 {
		t.Fatalf("2-3 örnek bekleniyordu, %d var", n)
	}

	db := testdb.Open(t)
	if _, err := seed.Run(t.Context(), db, now); err != nil {
		t.Fatal(err)
	}
	gen := &segment.Generator{LLM: llm.NewMock()}

	seen := map[string]bool{}
	for _, ex := range examples {
		t.Run(ex.ID, func(t *testing.T) {
			if ex.ID == "" || ex.Title == "" || ex.Brief == "" || seen[ex.ID] {
				t.Fatalf("eksik veya yinelenen alan: %+v", ex)
			}
			seen[ex.ID] = true

			seg, err := gen.Generate(t.Context(), ex.Brief)
			if err != nil {
				t.Fatalf("örnek brief desteklenmiyor: %v", err)
			}
			got, err := audience.Count(t.Context(), db, seg, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Total == 0 || got.SMS == 0 || got.Email == 0 {
				t.Fatalf("örnek brief demo verisinde yeterli alıcı bulamadı: %+v", got)
			}
			t.Logf("%s: toplam %d, SMS %d, e-posta %d", ex.ID, got.Total, got.SMS, got.Email)
		})
	}
}
