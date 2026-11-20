<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import {
  ArrowRightIcon,
  CalendarClockIcon,
  CirclePlusIcon,
  ClockIcon,
  RefreshCwIcon,
  SendIcon,
  TimerOffIcon,
} from '@lucide/vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { listScheduled, type ScheduledPage } from '@/api/scheduled'
import { errorTitle } from '@/api/messages'
import { totalPages } from '@/audience/members'
import { campaignTarget } from '@/campaign/links'
import { clockOffset, formatRemaining, msUntil, startTimeText } from '@/scheduled/time'
import ErrorPanel from '@/components/ErrorPanel.vue'
import ListPagination from '@/components/ListPagination.vue'
import PageHeader from '@/components/PageHeader.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'

// Zamanlanmış gönderimler: onaylanmış, gönderimi ileri bir zamana planlanmış ve henüz başlamamış
// kampanyalar, en yakın zamanı başta. Kalan süre SUNUCU saatine göre hesaplanır (sayfa her okunuşta
// sunucu saatini getirir; cihaz saati yanlış olsa da sayaç doğru kalır) ve dakikada bir tazelenir.
// Liste 30 sn'de bir sessizce yeniden okunur: gönderimi başlayan kampanya listeden kendiliğinden
// çıkar. Planlanan saat gönderimin başlayabileceği EN ERKEN andır.
const TICK_MS = 15_000
const REFRESH_MS = 30_000
const nf = new Intl.NumberFormat('tr-TR')

const page = ref(1)
const loading = ref(true)
const error = ref<ApiError | null>(null)
const data = ref<ScheduledPage | null>(null)
const offset = ref(0)
const clientNow = ref(Date.now())

let controller: AbortController | null = null
let tickTimer: ReturnType<typeof setInterval> | null = null
let refreshTimer: ReturnType<typeof setInterval> | null = null
/** Hata sonrası "Yeniden dene" için son istenen sayfa. */
let requested = 1

/** `silent`: yenileme yükleme durumu göstermez ve hatada eski listeyi korur. */
async function load(target: number, silent = false) {
  // Önceki istek sürüyorsa iptal et; geç gelen yanıt sonucu ezmemeli.
  controller?.abort()
  const current = new AbortController()
  controller = current
  requested = target
  if (!silent) {
    loading.value = true
    error.value = null
  }
  try {
    const result = await listScheduled(target, current.signal)
    if (controller !== current) return
    const last = totalPages(result.total, result.limit)
    if (result.items.length === 0 && result.total > 0 && target > last) {
      // Liste kısaldı (kampanyalar başladı): boş bir sayfa göstermek yerine son sayfaya git.
      await load(last, silent)
      return
    }
    data.value = result
    page.value = target
    offset.value = clockOffset(result.server_time, Date.now())
    clientNow.value = Date.now()
    error.value = null
  } catch (err) {
    if (controller !== current || isAbortError(err)) return
    if (silent && data.value) return
    error.value =
      err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
  } finally {
    if (controller === current) loading.value = false
  }
}

onMounted(() => {
  void load(1)
  tickTimer = setInterval(() => (clientNow.value = Date.now()), TICK_MS)
  refreshTimer = setInterval(() => void load(page.value, true), REFRESH_MS)
})
onBeforeUnmount(() => {
  if (tickTimer) clearInterval(tickTimer)
  if (refreshTimer) clearInterval(refreshTimer)
  controller?.abort()
})

const rows = computed(() =>
  (data.value?.items ?? []).map((item) => {
    const remaining = msUntil(item.scheduled_at, clientNow.value, offset.value)
    // Sunucu "zamanı geldi" demese bile sayaç sıfırı geçtiyse beklemeyi bırakıp bunu gösteririz.
    const due = item.due || remaining <= 0
    return {
      ...item,
      due,
      remaining: due ? '' : formatRemaining(remaining),
      startText: startTimeText(item.scheduled_at),
      to: campaignTarget(item.id, 'approved'),
    }
  }),
)
const lastPage = computed(() => (data.value ? totalPages(data.value.total, data.value.limit) : 1))

function goTo(target: number) {
  if (loading.value || target < 1 || target > lastPage.value || target === page.value) return
  void load(target)
}

const countdownTone = 'bg-sky-100 text-sky-900 dark:bg-sky-400/15 dark:text-sky-300'
const dueTone = 'bg-amber-100 text-amber-900 dark:bg-amber-400/15 dark:text-amber-300'
</script>

<template>
  <main class="grid gap-8">
    <PageHeader
      :icon="CalendarClockIcon"
      title="Zamanlanmış gönderimler"
      description="Gönderimi ileri bir zamana planlanmış ve henüz başlamamış kampanyalar. Saatler cihazınızın saat dilimindedir."
    />

    <div class="flex flex-wrap items-center justify-between gap-3">
      <!-- Sayı yalnızca bekleyen kampanya varken gösterilir; boşken boş durum zaten söyler ("0 kampanya
           bekliyor" gürültüdür). Bölge her zaman var: sayı değişince ekran okuyucuya bildirilir. -->
      <div aria-live="polite">
        <p
          v-if="data && data.total > 0"
          class="flex items-center gap-2.5 text-sm"
          data-testid="scheduled-total"
        >
          <span
            class="bg-primary/10 text-primary grid h-7 min-w-7 place-items-center rounded-md px-2 font-semibold tabular-nums"
          >
            {{ nf.format(data.total) }}
          </span>
          <span class="text-muted-foreground">{{ ' ' }}kampanya bekliyor</span>
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        size="lg"
        class="ml-auto"
        :disabled="loading"
        data-testid="scheduled-refresh"
        @click="load(page)"
      >
        <RefreshCwIcon aria-hidden="true" />
        Yenile
      </Button>
    </div>

    <Card>
      <CardContent class="grid gap-4">
        <div
          v-if="loading && !data"
          class="grid gap-3"
          role="status"
          data-testid="scheduled-loading"
        >
          <span class="sr-only">Zamanlanmış gönderimler yükleniyor…</span>
          <Skeleton v-for="n in 3" :key="n" class="h-16 w-full" />
        </div>

        <ErrorPanel
          v-else-if="error"
          :title="`Zamanlanmış gönderimler yüklenemedi. ${errorTitle(error)}`"
          :reasons="[]"
          :retryable="error.retryable"
          @retry="load(requested)"
        />

        <Empty v-else-if="data && data.total === 0" class="border" data-testid="scheduled-empty">
          <EmptyHeader>
            <EmptyMedia variant="icon"><TimerOffIcon aria-hidden="true" /></EmptyMedia>
            <EmptyTitle>Zamanlanmış gönderim yok</EmptyTitle>
            <EmptyDescription>
              Onay sırasında "tarih ve saat seç" ile planladığınız kampanyalar, gönderimi başlayana
              kadar burada görünür.
            </EmptyDescription>
          </EmptyHeader>
          <Button as-child variant="outline">
            <RouterLink to="/">
              <CirclePlusIcon aria-hidden="true" />
              Yeni kampanya
            </RouterLink>
          </Button>
        </Empty>

        <template v-else-if="data">
          <ul
            class="divide-y"
            :aria-busy="loading"
            :class="{ 'opacity-60': loading }"
            data-testid="scheduled-list"
          >
            <li
              v-for="row in rows"
              :key="row.id"
              class="grid gap-3 py-4 first:pt-0 last:pb-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
              data-testid="scheduled-row"
              :data-due="row.due"
            >
              <div class="grid min-w-0 gap-2">
                <p
                  class="line-clamp-2 text-sm font-medium wrap-anywhere"
                  data-testid="scheduled-brief"
                >
                  {{ row.brief }}
                </p>
                <p
                  class="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs"
                >
                  <span class="inline-flex items-center gap-1.5" data-testid="scheduled-start">
                    <CalendarClockIcon class="size-3.5" aria-hidden="true" />
                    {{ row.startText }}
                  </span>
                  <span class="inline-flex items-center gap-1.5" data-testid="scheduled-queued">
                    <SendIcon class="size-3.5" aria-hidden="true" />
                    {{ nf.format(row.queued) }} mesaj kuyrukta
                  </span>
                </p>
              </div>

              <div class="flex flex-wrap items-center gap-3 sm:justify-end">
                <Badge
                  variant="outline"
                  :class="['border-transparent', row.due ? dueTone : countdownTone]"
                  data-testid="scheduled-countdown"
                >
                  <ClockIcon aria-hidden="true" />
                  <template v-if="row.due">Zamanı geldi, işleniyor</template>
                  <template v-else>{{ row.remaining }} sonra başlayacak</template>
                </Badge>
                <Button as-child variant="outline" size="sm" data-testid="scheduled-open">
                  <RouterLink :to="row.to">
                    Gönderimi aç
                    <ArrowRightIcon aria-hidden="true" />
                  </RouterLink>
                </Button>
              </div>
            </li>
          </ul>

          <ListPagination
            :page="page"
            :total="data.total"
            :limit="data.limit"
            :loading="loading"
            @change="goTo"
          />
        </template>
      </CardContent>
    </Card>

    <p class="text-muted-foreground text-xs" data-testid="scheduled-note">
      Seçilen saat, gönderimin başlayabileceği en erken zamandır; bütün mesajların o anda bitmesi
      anlamına gelmez. Alıcılar onay anında belirlenir; kanal izni gönderimden hemen önce tekrar
      kontrol edilir.
    </p>
  </main>
</template>
