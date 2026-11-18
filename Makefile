.PHONY: up down test test-db e2e e2e-up e2e-down e2e-logs

up:
	docker compose up --build

down:
	docker compose down

# Entegrasyon testleri için yalnızca postgres servisini başlatır (ADR-015).
test-db:
	docker compose up -d --wait postgres

test: test-db
	cd backend && go test ./...

# Uçtan uca (Playwright) testler. Geliştirme ortamından ayrı, izole bir Compose
# projesinde (compose.e2e.yaml) koşar; geliştirme veritabanına dokunmaz. Yerelde
# ve CI'da aynıdır. `make test` e2e'yi içermez (hızlı kalsın, tarayıcı gerekmez).
E2E_COMPOSE = docker compose -f compose.yaml -f compose.e2e.yaml
E2E_BASE_URL ?= http://127.0.0.1:18080

e2e-up:
	$(E2E_COMPOSE) up -d --build --wait

e2e-down:
	$(E2E_COMPOSE) down -v --remove-orphans

e2e-logs:
	$(E2E_COMPOSE) logs --no-color --tail=200

# Ortamı kurar, testleri koşturur ve (başarılı da olsa başarısız da olsa) ortamı
# siler. Çıkış kodu testlerin çıkış kodudur. Test başarısız olursa servis
# logları yazdırılır. (`$(MAKE)` kullanılmaz: `make -n` hiçbir şey çalıştırmaz.)
e2e:
	@$(E2E_COMPOSE) up -d --build --wait || { $(E2E_COMPOSE) logs --no-color --tail=200; $(E2E_COMPOSE) down -v --remove-orphans; exit 1; }
	@(cd frontend && E2E_BASE_URL=$(E2E_BASE_URL) npx playwright test); rc=$$?; \
	  if [ $$rc -ne 0 ]; then $(E2E_COMPOSE) logs --no-color --tail=200; fi; \
	  $(E2E_COMPOSE) down -v --remove-orphans; exit $$rc
