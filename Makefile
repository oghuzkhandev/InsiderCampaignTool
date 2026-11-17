.PHONY: up down test test-db

up:
	docker compose up --build

down:
	docker compose down

# Entegrasyon testleri için yalnızca postgres servisini başlatır (ADR-015).
test-db:
	docker compose up -d --wait postgres

test: test-db
	cd backend && go test ./...
