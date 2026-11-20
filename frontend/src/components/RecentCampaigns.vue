<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { HistoryIcon } from '@lucide/vue'
import {
  ApiError,
  DEFAULT_LIST_LIMIT,
  isAbortError,
  listCampaigns,
  type CampaignSummary,
} from '@/api/campaigns'
import { errorTitle } from '@/api/messages'
import { campaignTarget, campaignTargetLabel } from '@/campaign/links'
import ErrorPanel from '@/components/ErrorPanel.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { Skeleton } from '@/components/ui/skeleton'

// "Son kampanyalar" (CMP-001): en yeni 10 kampanya, satırda brief, durum ve tarih. Kenar
// çubuğunda her ekranda görünür. Taslak kampanya içerik ve onay ekranına, onaylanmış kampanya
// gönderim takibine açılır. Liste ikincil bir bilgidir: yüklenemezse sayfanın geri kalanı
// çalışmaya devam eder.
const props = defineProps<{
  /** Değişince liste yeniden okunur (ör. yeni bir kampanya oluşunca veya başka ekrana geçilince). */
  reloadKey?: string | number | null
}>()

const loading = ref(true)
const error = ref<ApiError | null>(null)
const rows = ref<CampaignSummary[]>([])

let controller: AbortController | null = null

async function load() {
  controller?.abort()
  const current = new AbortController()
  controller = current
  loading.value = true
  error.value = null
  try {
    const result = await listCampaigns(DEFAULT_LIST_LIMIT, current.signal)
    if (controller !== current) return
    rows.value = result
  } catch (err) {
    if (controller !== current || isAbortError(err)) return
    error.value =
      err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
  } finally {
    if (controller === current) loading.value = false
  }
}

watch(() => props.reloadKey, load, { immediate: true })
onBeforeUnmount(() => controller?.abort())

const dateFormat = new Intl.DateTimeFormat('tr-TR', { dateStyle: 'medium', timeStyle: 'short' })
const dateText = (iso: string) => dateFormat.format(new Date(iso))
</script>

<template>
  <section aria-labelledby="recent-title" class="grid gap-2" data-testid="recent-campaigns">
    <h2
      id="recent-title"
      class="text-sidebar-foreground/70 flex items-center gap-2 px-2 text-xs font-medium"
    >
      <HistoryIcon class="size-4" aria-hidden="true" />
      Son kampanyalar
    </h2>

    <!-- Önceki liste varken yenilenirse (loading) eski satırlar görünür kalır; ilk yüklemede iskelet. -->
    <div
      v-if="loading && rows.length === 0"
      class="grid gap-2"
      role="status"
      data-testid="recent-loading"
    >
      <span class="sr-only">Son kampanyalar yükleniyor…</span>
      <Skeleton class="h-14 w-full" />
      <Skeleton class="h-14 w-full" />
      <Skeleton class="h-14 w-full" />
    </div>

    <ErrorPanel
      v-else-if="error"
      :title="`Son kampanyalar yüklenemedi. ${errorTitle(error)}`"
      :reasons="[]"
      :retryable="true"
      @retry="load"
    />

    <p
      v-else-if="rows.length === 0"
      class="text-sidebar-foreground/70 px-2 text-sm"
      data-testid="recent-empty"
    >
      Henüz kampanya yok. İlk kampanyanızı brief yazarak başlatın.
    </p>

    <ul v-else class="grid gap-1" data-testid="recent-list">
      <li v-for="row in rows" :key="row.id">
        <RouterLink
          :to="campaignTarget(row.id, row.status)"
          :title="campaignTargetLabel(row.status)"
          class="hover:bg-sidebar-accent aria-[current=page]:bg-sidebar-accent focus-visible:ring-sidebar-ring/60 grid gap-1.5 rounded-lg px-2 py-2 transition-colors outline-none focus-visible:ring-2"
          data-testid="recent-row"
          :data-status="row.status"
        >
          <span class="line-clamp-2 text-sm wrap-anywhere" data-testid="recent-brief">
            {{ row.brief }}
          </span>
          <span class="flex flex-wrap items-center gap-x-2 gap-y-1">
            <StatusBadge :status="row.status" data-testid="recent-status" />
            <time class="text-sidebar-foreground/70 text-xs" :datetime="row.created_at">
              {{ dateText(row.created_at) }}
            </time>
          </span>
          <!-- Satırın nereye açıldığı: görsel olarak durum rozeti anlatır, okuyucuya metin verilir. -->
          <span class="sr-only" data-testid="recent-target">
            {{ campaignTargetLabel(row.status) }}
          </span>
        </RouterLink>
      </li>
    </ul>
  </section>
</template>
