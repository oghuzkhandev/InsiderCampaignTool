.PHONY: up down test test-db e2e e2e-standard e2e-delivery e2e-observability e2e-metrics-internal metrics-lint e2e-up e2e-delivery-up e2e-down e2e-logs

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
#
# İki ayrı döngü vardır (her biri kendi taze ortamında):
#   e2e-standard  worker ve provider YOK; kampanya onaydan sonra `approved` kalır.
#                 `@real` etiketli testler dışarıda bırakılır.
#   e2e-delivery  `delivery` profili açık: gerçek worker + mock provider + SSE.
#                 Yalnızca `@real` etiketli testler koşar.
# `make e2e` ikisini sırayla koşturur.
E2E_COMPOSE = docker compose -f compose.yaml -f compose.e2e.yaml
# Kapatma ve loglar profili de kapsar; hangi döngü açık olursa olsun hiçbir
# container ve volume geride kalmaz.
E2E_COMPOSE_ALL = $(E2E_COMPOSE) --profile delivery --profile observability
# promtool, Prometheus imajıyla gelir; ayrı kurulum gerekmez (yalnızca Docker).
PROMTOOL_IMAGE ?= prom/prometheus:v3.5.0
E2E_BASE_URL ?= http://127.0.0.1:18080

e2e-up:
	$(E2E_COMPOSE) up -d --build --wait

e2e-delivery-up:
	$(E2E_COMPOSE) --profile delivery up -d --build --wait

e2e-down:
	$(E2E_COMPOSE_ALL) down -v --remove-orphans

e2e-logs:
	$(E2E_COMPOSE_ALL) logs --no-color --tail=200

# Ortamı kurar, testleri koşturur ve (başarılı da olsa başarısız da olsa) ortamı
# siler. Çıkış kodu testlerin çıkış kodudur. Test başarısız olursa servis
# logları yazdırılır.
e2e-standard:
	@$(E2E_COMPOSE_ALL) down -v --remove-orphans >/dev/null 2>&1; \
	$(E2E_COMPOSE) up -d --build --wait || { $(E2E_COMPOSE_ALL) logs --no-color --tail=200; $(E2E_COMPOSE_ALL) down -v --remove-orphans; exit 1; }
	@(cd frontend && E2E_BASE_URL=$(E2E_BASE_URL) npx playwright test --grep-invert @real); rc=$$?; \
	  if [ $$rc -ne 0 ]; then $(E2E_COMPOSE_ALL) logs --no-color --tail=200; fi; \
	  $(E2E_COMPOSE_ALL) down -v --remove-orphans; exit $$rc

e2e-delivery:
	@$(E2E_COMPOSE_ALL) down -v --remove-orphans >/dev/null 2>&1; \
	$(E2E_COMPOSE) --profile delivery up -d --build --wait || { $(E2E_COMPOSE_ALL) logs --no-color --tail=200; $(E2E_COMPOSE_ALL) down -v --remove-orphans; exit 1; }
	@$(MAKE) --no-print-directory e2e-metrics-internal && (cd frontend && E2E_BASE_URL=$(E2E_BASE_URL) npx playwright test --grep @real); rc=$$?; \
	  if [ $$rc -ne 0 ]; then $(E2E_COMPOSE_ALL) logs --no-color --tail=200; fi; \
	  $(E2E_COMPOSE_ALL) down -v --remove-orphans; exit $$rc

# `/metrics` yalnızca Docker iç ağından erişilir, host'a yayımlanmaz (ADR-016,
# TRK-001). Çalışan `delivery` ortamına karşı: iç ağdan (web container'ı) API ve
# worker metrikleri okunur, API ana portu /metrics sunmaz ve api ile worker hiçbir
# portu host'a yayımlamaz.
e2e-metrics-internal:
	@$(E2E_COMPOSE_ALL) exec -T web wget -qO- http://api:9090/metrics | grep -q '^queue_depth{channel="sms"}' \
	  || { echo "HATA: API /metrics iç ağdan okunamadı"; exit 1; }
	@$(E2E_COMPOSE_ALL) exec -T web wget -qO- http://worker:9090/metrics | grep -q '^messages_sent_total' \
	  || { echo "HATA: worker /metrics iç ağdan okunamadı"; exit 1; }
	@if $(E2E_COMPOSE_ALL) exec -T web wget -qO- http://api:8081/metrics >/dev/null 2>&1; then \
	  echo "HATA: API ana portu /metrics sunmamalı"; exit 1; fi
	@for s in api worker; do \
	  pub="$$($(E2E_COMPOSE_ALL) ps $$s --format '{{range .Publishers}}{{if .PublishedPort}}{{.TargetPort}} {{end}}{{end}}')"; \
	  case "$$pub" in *9090*) echo "HATA: $$s /metrics portunu host'a yayımlıyor: $$pub"; exit 1;; esac; \
	done
	@echo "metrics: iç ağdan erişilir, host'a yayımlanmaz (tamam)"

# Metrik çıktısını `promtool` ile denetler (Prometheus sunucusu gerekmez, yalnızca
# `promtool` imajı): çalışan `delivery` + `observability` ortamında API ve worker
# `/metrics` çıktıları `promtool check metrics`, `observability/prometheus.yml`
# ise `promtool check config` ile doğrulanır. Boş çıktı hata sayılır (promtool boş
# girdide sessizce geçer).
metrics-lint:
	@for t in api worker; do \
	  out="$$($(E2E_COMPOSE_ALL) exec -T prometheus wget -qO- http://$$t:9090/metrics)" \
	    || { echo "HATA: $$t /metrics okunamadı"; exit 1; }; \
	  [ -n "$$out" ] || { echo "HATA: $$t /metrics boş"; exit 1; }; \
	  printf '%s\n' "$$out" | docker run --rm -i --entrypoint promtool $(PROMTOOL_IMAGE) check metrics \
	    || { echo "HATA: $$t metrik çıktısı promtool kontrolünden geçmedi"; exit 1; }; \
	  echo "promtool check metrics: $$t temiz"; \
	done
	@docker run --rm --entrypoint promtool -v "$(CURDIR)/observability:/cfg:ro" $(PROMTOOL_IMAGE) \
	  check config /cfg/prometheus.yml

# İsteğe bağlı Prometheus (`observability` profili): önce `promtool` denetimi, sonra
# gerçek scrape'te api ve worker `up`, gerçek metrikler geliyor, saklama sınırlı ve
# arayüz yalnızca 127.0.0.1'de; en son worker durdurulup `up=0` doğrulanır (bu yüzden
# promtool denetimi önce koşar). Kendi ortamını kurar ve siler.
e2e-observability:
	@$(E2E_COMPOSE_ALL) down -v --remove-orphans >/dev/null 2>&1; \
	$(E2E_COMPOSE_ALL) up -d --build --wait || { $(E2E_COMPOSE_ALL) logs --no-color --tail=200; $(E2E_COMPOSE_ALL) down -v --remove-orphans; exit 1; }
	@pub="$$($(E2E_COMPOSE_ALL) ps prometheus --format '{{range .Publishers}}{{if .PublishedPort}}{{.URL}}:{{.PublishedPort}} {{end}}{{end}}')"; \
	  echo "prometheus yayımlanan: $$pub"; \
	  [ "$$pub" = "127.0.0.1:19091 " ] || { echo "HATA: Prometheus arayüzü yalnızca 127.0.0.1:19091'de yayımlanmalı: [$$pub]"; rc=1; }; \
	  [ "$${rc:-0}" = 0 ] && $(MAKE) --no-print-directory metrics-lint \
	    && PROM_URL=http://127.0.0.1:19091 COMPOSE="$(E2E_COMPOSE_ALL)" sh observability/verify.sh; rc=$$?; \
	  if [ $$rc -ne 0 ]; then $(E2E_COMPOSE_ALL) logs --no-color --tail=200; fi; \
	  $(E2E_COMPOSE_ALL) down -v --remove-orphans; exit $$rc

# Üç döngüyü de koşturur; biri kırılsa bile diğerleri çalışır, çıkış kodu
# herhangi birinin başarısızlığını yansıtır. (`$(MAKE)` satırları `make -n` ile de
# genişler; kuru çalıştırma hiçbir şey başlatmaz.)
e2e:
	@rc=0; \
	$(MAKE) --no-print-directory e2e-standard || rc=1; \
	$(MAKE) --no-print-directory e2e-delivery || rc=1; \
	$(MAKE) --no-print-directory e2e-observability || rc=1; \
	exit $$rc
