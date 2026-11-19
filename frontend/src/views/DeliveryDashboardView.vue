<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { ApiError, isAbortError } from '@/api/campaigns'
import { getCampaign, type CampaignDetail } from '@/api/content'
import { contentErrorTitle } from '@/content/messages'
import { campaignStatusLabel, campaignStatusVariant, deliveryErrorTitle } from '@/delivery/messages'
import { useDeliveryStream } from '@/delivery/useDeliveryStream'
import DeliveryConnectionStatus from '@/components/DeliveryConnectionStatus.vue'
import DeliveryCounts from '@/components/DeliveryCounts.vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

const props = defineProps<{ id: string }>()

const loading = ref(true)
const loadError = ref<ApiError | null>(null)
const detail = ref<CampaignDetail | null>(null)

let loadController: AbortController | null = null

async function load() {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  // Önceki kampanyanın bilgisi hemen bırakılır: canlı takip bağlantısı da buna bağlı
  // olarak kapanır ve yeni kampanyada eski sayılar görünmez.
  detail.value = null
  loadError.value = null
  loading.value = true
  try {
    const result = await getCampaign(props.id, controller.signal)
    if (loadController !== controller) return
    detail.value = result
  } catch (err) {
    if (loadController !== controller || isAbortError(err)) return
    loadError.value =
      err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
  } finally {
    if (loadController === controller) loading.value = false
  }
}

watch(() => props.id, load, { immediate: true })
onBeforeUnmount(() => loadController?.abort())

const isDraft = computed(() => detail.value?.status === 'draft')
// Onaylanmamış kampanyada gönderim yoktur; canlı takibe yalnızca onaylı kampanyada bağlanılır.
const streamTarget = computed(() => (detail.value && !isDraft.value ? props.id : null))
const { snapshot, connection, failures, error, updatedAt, retry } = useDeliveryStream(
  () => streamTarget.value,
)

// Canlı görüntü varsa kampanya durumu oradan (güncel), yoksa yüklenen kayıttan gelir.
const status = computed(() => snapshot.value?.status ?? detail.value?.status ?? '')
const approvedAtText = computed(() => {
  const at = detail.value?.approved_at
  return at ? new Date(at).toLocaleString('tr-TR') : ''
})
const streamErrorTitle = computed(() => (error.value ? deliveryErrorTitle(error.value) : null))
</script>

<template>
  <main class="grid gap-6">
    <Button as-child variant="link" class="justify-self-start px-0">
      <RouterLink :to="`/campaigns/${id}`">← Kampanyaya dön</RouterLink>
    </Button>
    <h1 class="text-2xl font-bold">Gönderim takibi</h1>

    <div v-if="loading" class="grid gap-3" role="status" data-testid="loading">
      <span class="sr-only">Kampanya yükleniyor…</span>
      <Skeleton class="h-5 w-2/3" />
      <Skeleton class="h-40 w-full" />
    </div>

    <Alert v-else-if="loadError" variant="destructive" data-testid="load-error">
      <AlertTitle>{{ contentErrorTitle(loadError) }}</AlertTitle>
      <AlertDescription>
        <Button v-if="loadError.retryable" type="button" variant="outline" size="sm" @click="load">
          Yeniden dene
        </Button>
      </AlertDescription>
    </Alert>

    <template v-else-if="detail">
      <section class="grid gap-3" aria-labelledby="campaign-title">
        <h2 id="campaign-title" class="text-lg font-bold">Kampanya</h2>
        <p class="text-muted-foreground text-sm [overflow-wrap:anywhere] whitespace-pre-wrap">
          {{ detail.brief }}
        </p>
        <div class="flex flex-wrap items-center gap-2">
          <Badge :variant="campaignStatusVariant(status)" data-testid="campaign-status">
            {{ campaignStatusLabel(status) }}
          </Badge>
          <span v-if="approvedAtText" class="text-muted-foreground text-sm">
            Onay: {{ approvedAtText }}
          </span>
        </div>
      </section>

      <Alert v-if="isDraft" data-testid="not-approved">
        <AlertTitle>Bu kampanya henüz onaylanmadı.</AlertTitle>
        <AlertDescription>
          Gönderim, kampanya onaylandıktan sonra başlar. İçeriği hazırlayıp onaylamak için
          kampanyaya dönün.
        </AlertDescription>
      </Alert>

      <template v-else>
        <DeliveryConnectionStatus
          :connection="connection"
          :failures="failures"
          :updated-at="updatedAt"
          :error-title="streamErrorTitle"
          @retry="retry"
        />

        <DeliveryCounts v-if="snapshot" :snapshot="snapshot" />
        <div
          v-else-if="connection !== 'failed'"
          class="grid gap-3"
          data-testid="waiting-first-snapshot"
        >
          <Skeleton class="h-40 w-full" />
        </div>

        <Alert v-if="connection === 'completed'" data-testid="completed-note">
          <AlertTitle>Gönderim tamamlandı.</AlertTitle>
          <AlertDescription>
            Bütün mesajların işi bitti. Başarısız veya atlanan mesajlar olması kampanyanın
            tamamlanmasını engellemez; yukarıdaki sayılar gerçek dağılımı gösterir.
          </AlertDescription>
        </Alert>
      </template>
    </template>
  </main>
</template>
