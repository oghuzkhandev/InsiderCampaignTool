<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CircleAlertIcon, CircleCheckBigIcon, FileTextIcon, InfoIcon } from '@lucide/vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { getCampaign, type CampaignDetail } from '@/api/content'
import { contentErrorTitle } from '@/content/messages'
import { completionHeadline, completionLines, deliveryErrorTitle } from '@/delivery/messages'
import { useDeliveryStream } from '@/delivery/useDeliveryStream'
import DeliveryConnectionStatus from '@/components/DeliveryConnectionStatus.vue'
import DeliveryCounts from '@/components/DeliveryCounts.vue'
import CampaignSteps from '@/components/CampaignSteps.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import StepNav, { type NavStep } from '@/components/StepNav.vue'
import { notifyCampaignsChanged } from '@/campaign/recent'
import { stepStatuses } from '@/campaign/steps'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
// Gönderim ilerleyip kampanya durumu değişince (ör. gönderiliyor → tamamlandı) kenar çubuğundaki
// liste de güncellensin. İlk yüklemedeki '' → durum geçişi bir değişiklik sayılmaz.
watch(status, (now, before) => {
  if (before && now !== before) notifyCampaignsChanged()
})
// Bu ekrana yalnızca onaylı kampanyada adım gösterilir; ilk üç adım o zaman bitmiştir.
const steps = computed(() => stepStatuses({ status: status.value, contentReady: true }))

// "Geri": kampanyaya (içerik ve onay). "İleri": son aşama olduğu için yeni bir kampanya
// başlatmak; gönderimin bitmesine bağlı değildir. Gönderim sunucuda (worker) sürer: bu sayfadan
// ayrılmak onu durdurmaz, kampanyanın durumu sonradan buradan izlenebilir.
const backStep = computed<NavStep>(() => ({
  label: 'Kampanyaya dön',
  to: `/campaigns/${props.id}`,
}))
const nextStep = computed<NavStep>(() => ({
  label: 'Yeni kampanya',
  to: '/',
  // Gönderim sürerken kullanıcı ayrılırsa bunun gönderimi durdurmadığını bilsin.
  hint: status.value === 'completed' ? undefined : 'Gönderim arka planda devam eder.',
}))
const approvedAtText = computed(() => {
  const at = detail.value?.approved_at
  return at ? new Date(at).toLocaleString('tr-TR') : ''
})
const streamErrorTitle = computed(() => (error.value ? deliveryErrorTitle(error.value) : null))
</script>

<template>
  <main class="mx-auto grid w-full max-w-3xl gap-6">
    <PageHeader title="Gönderim takibi" description="Mesajların durumu canlı güncellenir.">
      <template #badge>
        <StatusBadge v-if="detail" :status="status" data-testid="campaign-status" />
      </template>
      <CampaignSteps v-if="detail && !isDraft" :statuses="steps" />
    </PageHeader>

    <div v-if="loading" class="grid gap-3" role="status" data-testid="loading">
      <span class="sr-only">Kampanya yükleniyor…</span>
      <Skeleton class="h-5 w-2/3" />
      <Skeleton class="h-40 w-full" />
    </div>

    <Alert v-else-if="loadError" variant="destructive" data-testid="load-error">
      <CircleAlertIcon aria-hidden="true" />
      <AlertTitle>{{ contentErrorTitle(loadError) }}</AlertTitle>
      <AlertDescription>
        <Button v-if="loadError.retryable" type="button" variant="outline" size="sm" @click="load">
          Yeniden dene
        </Button>
      </AlertDescription>
    </Alert>

    <template v-else-if="detail">
      <Card role="region" aria-labelledby="campaign-title">
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <FileTextIcon class="text-primary size-5" aria-hidden="true" />
            <h2 id="campaign-title">Kampanya</h2>
          </CardTitle>
        </CardHeader>
        <CardContent class="grid gap-3">
          <p class="text-sm wrap-anywhere whitespace-pre-wrap">
            {{ detail.brief }}
          </p>
          <p v-if="approvedAtText" class="text-muted-foreground text-sm">
            Onay: {{ approvedAtText }}
          </p>
        </CardContent>
      </Card>

      <Alert v-if="isDraft" data-testid="not-approved">
        <InfoIcon aria-hidden="true" />
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
          <CircleCheckBigIcon aria-hidden="true" />
          <AlertTitle>Gönderim tamamlandı.</AlertTitle>
          <AlertDescription>
            <template v-if="snapshot">
              <p data-testid="completion-headline">{{ completionHeadline(snapshot.messages) }}</p>
              <ul class="grid list-disc gap-1 pl-5" data-testid="completion-lines">
                <li v-for="line in completionLines(snapshot)" :key="line.key">
                  <strong class="text-foreground">{{ line.label }}: {{ line.count }}</strong>
                  — {{ line.text }}
                  <div v-if="line.advice.length > 0" class="mt-1" data-testid="completion-advice">
                    <p class="text-foreground text-xs font-medium">Ne yapabilirsiniz?</p>
                    <ul class="grid list-[circle] gap-1 pl-5 text-xs">
                      <li v-for="(item, i) in line.advice" :key="i">{{ item }}</li>
                    </ul>
                  </div>
                </li>
              </ul>
            </template>
            <p>
              Bütün mesajların işi bitti. Başarısız veya atlanan mesajlar olması kampanyanın
              tamamlanmasını engellemez; yukarıdaki sayılar gerçek dağılımı gösterir.
            </p>
          </AlertDescription>
        </Alert>
      </template>
    </template>

    <StepNav :back="backStep" :next="nextStep" />
  </main>
</template>
