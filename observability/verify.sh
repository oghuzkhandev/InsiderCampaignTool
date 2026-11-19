#!/usr/bin/env sh
# Çalışan e2e ortamında (compose.e2e.yaml, profiller: delivery + observability)
# Prometheus'un GERÇEK scrape yaptığını doğrular. `make e2e-observability` çağırır.
#
#   1. api ve worker hedefleri up (=1)
#   2. gerçek metrikler geliyor: queue_depth{sms,email} (api), messages_sent_total (worker)
#   3. saklama sınırlı: 2 gün ve 256MB
#   4. worker durunca up{job="worker"}=0 olur, api hedefi ve queue_depth görünmeye devam
#      eder (worker durduğunda da kuyruk birikimi izlenebilir: ADR-016)
#
# Ortam: PROM_URL (varsayılan http://127.0.0.1:19091), COMPOSE (worker'ı durdurmak
# için docker compose komutu).
set -eu

PROM="${PROM_URL:-http://127.0.0.1:19091}"
COMPOSE="${COMPOSE:-docker compose}"

fail() {
  echo "HATA: $*" >&2
  curl -fsS --max-time 5 "$PROM/api/v1/targets" 2>/dev/null \
    | jq -r '.data.activeTargets[] | "  hedef \(.labels.job) \(.scrapeUrl): \(.health) \(.lastError)"' >&2 || true
  exit 1
}

# query İFADE: anlık sorgunun ilk sonucunun değerini yazar (sonuç yoksa boş).
query() {
  curl -fsS --max-time 5 -G "$PROM/api/v1/query" --data-urlencode "query=$1" \
    | jq -r '.data.result[0].value[1] // empty'
}

# until DEĞER İFADE ZAMAN: ifade DEĞER olana kadar bekler.
until_value() {
  want="$1"; expr="$2"; max="$3"; i=0
  while [ "$i" -lt "$max" ]; do
    got="$(query "$expr" 2>/dev/null || true)"
    [ "$got" = "$want" ] && return 0
    i=$((i + 1)); sleep 2
  done
  return 1
}

echo "Prometheus hazır olması bekleniyor ($PROM)..."
i=0
until curl -fsS --max-time 3 "$PROM/-/ready" >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -lt 30 ] || fail "Prometheus hazır olmadı"
  sleep 2
done

# 1) İki hedef de up.
until_value 2 'count(up{job=~"api|worker"} == 1)' 45 \
  || fail "api ve worker hedeflerinin ikisi de up olmadı"
echo "tamam: api ve worker hedefleri up"

# 2) Gerçek metrikler.
[ "$(query 'count(queue_depth{job="api"})')" = "2" ] \
  || fail "api queue_depth{channel} serileri (sms, email) gelmedi"
[ -n "$(query 'messages_sent_total{job="worker"}')" ] \
  || fail "worker messages_sent_total gelmedi"
[ -n "$(query 'ratelimit_wait_seconds_count{job="worker"}')" ] \
  || fail "worker ratelimit_wait_seconds gelmedi"
# API worker sayaçlarını sunmaz (ADR-016).
[ -z "$(query 'messages_sent_total{job="api"}')" ] \
  || fail "api çıktısında worker sayacı var"
echo "tamam: queue_depth (api), worker sayaçları ve histogram geliyor; api worker sayacı sunmuyor"

# 3) Saklama sınırlı.
flags="$(curl -fsS --max-time 5 "$PROM/api/v1/status/flags")"
[ "$(echo "$flags" | jq -r '.data["storage.tsdb.retention.time"]')" = "2d" ] \
  || fail "saklama süresi 2d değil"
# Prometheus `256MB` bayrağını `256MiB` olarak gösterir.
[ "$(echo "$flags" | jq -r '.data["storage.tsdb.retention.size"]')" = "256MiB" ] \
  || fail "saklama boyutu 256MB (256MiB) değil"
echo "tamam: saklama 2 gün / 256MB"

# 4) Worker durunca up=0, api ve queue_depth sürer.
$COMPOSE stop worker >/dev/null 2>&1 || fail "worker durdurulamadı"
until_value 0 'up{job="worker"}' 30 || fail "worker durunca up{job=\"worker\"} 0 olmadı"
[ "$(query 'up{job="api"}')" = "1" ] || fail "worker durunca api hedefi up kalmalı"
[ "$(query 'count(queue_depth{job="api"})')" = "2" ] \
  || fail "worker durunca queue_depth görünmeye devam etmeli"
echo "tamam: worker durunca up=0, api ve queue_depth görünmeye devam ediyor"
echo "observability doğrulaması başarılı"
