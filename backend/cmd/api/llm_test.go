package main

import (
	"testing"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
)

func TestNewLLMDefaultsToMock(t *testing.T) {
	for _, provider := range []string{"", "mock"} {
		client, err := newLLM(provider)
		if err != nil {
			t.Fatalf("%q: %v", provider, err)
		}
		if _, ok := client.(*llm.Mock); !ok {
			t.Fatalf("%q için MockLLM bekleniyordu: %T", provider, client)
		}
	}
}

func TestNewLLMRejectsUnsupportedProvider(t *testing.T) {
	if _, err := newLLM("anthropic"); err == nil {
		t.Fatal("henüz desteklenmeyen sağlayıcı sessizce kabul edilmemeli")
	}
}
