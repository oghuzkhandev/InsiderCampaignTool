<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ChevronLeftIcon, ChevronRightIcon, UsersRoundIcon } from '@lucide/vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { listAudienceMembers, type AudiencePage } from '@/api/audience'
import { errorTitle } from '@/api/messages'
import { AUDIENCE_LIST_NOTE, pageRange, totalPages } from '@/audience/members'
import ErrorPanel from '@/components/ErrorPanel.vue'
import MemberTable from '@/components/MemberTable.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'

// "Segmente uyan kişiler" (ADR-031): kampanyanın kayıtlı segmentine uyan kişilerin sayfalı listesi.
// Kanal iznine bakılmadan eşleşen herkes görünür; izni olmayan kişi listelenir ama ilgili kanal için
// "uygun değil" gösterilir. Liste ikincil bir bilgidir: yüklenemezse sayfanın geri kalanı çalışır.
const props = defineProps<{ campaignId: string }>()

const page = ref(1)
const loading = ref(true)
const error = ref<ApiError | null>(null)
const data = ref<AudiencePage | null>(null)

let controller: AbortController | null = null
/** Hata sonrası "Yeniden dene" için son istenen sayfa. */
let requested = 1

async function load(target: number) {
  // Önceki istek sürüyorsa iptal et; geç gelen yanıt sonucu ezmemeli.
  controller?.abort()
  const current = new AbortController()
  controller = current
  requested = target
  loading.value = true
  error.value = null
  try {
    const result = await listAudienceMembers(props.campaignId, target, current.signal)
    if (controller !== current) return
    const last = totalPages(result.total, result.limit)
    if (result.items.length === 0 && result.total > 0 && target > last) {
      // Liste kısaldı (ör. izinler veya tarih değişti): boş bir sayfa göstermek yerine sonuncuya git.
      await load(last)
      return
    }
    data.value = result
    page.value = target
  } catch (err) {
    if (controller !== current || isAbortError(err)) return
    error.value =
      err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
  } finally {
    if (controller === current) loading.value = false
  }
}

// Kampanya değişirse (yeni brief) liste baştan okunur; eski kampanyanın satırları kalmaz.
watch(
  () => props.campaignId,
  () => {
    data.value = null
    page.value = 1
    void load(1)
  },
  { immediate: true },
)
onBeforeUnmount(() => controller?.abort())

const lastPage = computed(() => (data.value ? totalPages(data.value.total, data.value.limit) : 1))
const range = computed(() =>
  data.value ? pageRange(page.value, data.value.limit, data.value.items.length) : null,
)
const nf = new Intl.NumberFormat('tr-TR')

function goTo(target: number) {
  if (loading.value || target < 1 || target > lastPage.value || target === page.value) return
  void load(target)
}
</script>

<template>
  <Card role="region" aria-labelledby="audience-members-title" data-testid="audience-members">
    <CardHeader>
      <CardTitle class="flex items-center gap-2">
        <UsersRoundIcon class="text-primary size-5" aria-hidden="true" />
        <h2 id="audience-members-title">Segmente uyan kişiler</h2>
      </CardTitle>
      <CardDescription data-testid="audience-members-note">
        {{ AUDIENCE_LIST_NOTE }}
      </CardDescription>
    </CardHeader>

    <CardContent class="grid gap-4">
      <!-- İlk yüklemede iskelet. Sonraki sayfalarda eski satırlar görünür kalır (aria-busy). -->
      <div
        v-if="loading && !data"
        class="grid gap-2"
        role="status"
        data-testid="audience-members-loading"
      >
        <span class="sr-only">Kişi listesi yükleniyor…</span>
        <Skeleton v-for="n in 5" :key="n" class="h-12 w-full" />
      </div>

      <ErrorPanel
        v-else-if="error"
        :title="`Kişi listesi yüklenemedi. ${errorTitle(error)}`"
        :reasons="[]"
        :retryable="error.retryable"
        @retry="load(requested)"
      />

      <Empty
        v-else-if="data && data.total === 0"
        class="border"
        data-testid="audience-members-empty"
      >
        <EmptyHeader>
          <EmptyMedia variant="icon"><UsersRoundIcon aria-hidden="true" /></EmptyMedia>
          <EmptyTitle>Bu tanımla eşleşen kişi yok</EmptyTitle>
          <EmptyDescription>
            Segment hiçbir kullanıcıyla eşleşmedi. Brief'i düzenleyip kitleyi yeniden hesaplayın.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>

      <template v-else-if="data">
        <div
          :aria-busy="loading"
          :class="{ 'opacity-60': loading }"
          data-testid="audience-members-page"
        >
          <MemberTable
            :members="data.items"
            :caption="`Segmente uyan kişiler, kimliğe göre sıralı; sayfa ${page} / ${lastPage}`"
            data-testid="audience-members-table"
          />
        </div>

        <nav
          aria-label="Kişi listesi sayfalama"
          class="flex flex-wrap items-center justify-between gap-3"
          data-testid="audience-members-pager"
        >
          <p
            class="text-muted-foreground text-sm"
            aria-live="polite"
            data-testid="audience-members-status"
          >
            <template v-if="range">
              {{ nf.format(range.from) }}–{{ nf.format(range.to) }} /
              {{ nf.format(data.total) }} kişi · Sayfa {{ nf.format(page) }} /
              {{ nf.format(lastPage) }}
            </template>
            <template v-else>Sayfa {{ nf.format(page) }} / {{ nf.format(lastPage) }}</template>
          </p>
          <div class="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="lg"
              :disabled="loading || page <= 1"
              data-testid="audience-members-prev"
              @click="goTo(page - 1)"
            >
              <ChevronLeftIcon aria-hidden="true" />
              Önceki
            </Button>
            <Button
              type="button"
              variant="outline"
              size="lg"
              :disabled="loading || page >= lastPage"
              data-testid="audience-members-next"
              @click="goTo(page + 1)"
            >
              Sonraki
              <ChevronRightIcon aria-hidden="true" />
            </Button>
          </div>
        </nav>
      </template>
    </CardContent>
  </Card>
</template>
