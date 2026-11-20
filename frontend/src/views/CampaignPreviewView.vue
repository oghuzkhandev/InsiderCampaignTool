<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { ApiError, createCampaign, isAbortError, type Campaign } from '@/api/campaigns'
import { errorTitle } from '@/api/messages'
import { reasonMessage } from '@/segment/reasons'
import AudienceCounts from '@/components/AudienceCounts.vue'
import AudienceMemberList from '@/components/AudienceMemberList.vue'
import BriefForm from '@/components/BriefForm.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import SegmentSummary from '@/components/SegmentSummary.vue'
import CampaignSteps from '@/components/CampaignSteps.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import StepNav, { type NavStep } from '@/components/StepNav.vue'
import { notifyCampaignsChanged } from '@/campaign/recent'
import { stepStatuses } from '@/campaign/steps'
import { Card, CardContent } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'

const brief = ref('')
const loading = ref(false)
const campaign = ref<Campaign | null>(null)
const error = ref<ApiError | null>(null)

let lastSubmitted = ''
let controller: AbortController | null = null

async function submit(text: string) {
  // Önceki istek sürüyorsa iptal et; geç gelen yanıt sonucu ezmemeli.
  controller?.abort()
  const current = new AbortController()
  controller = current

  lastSubmitted = text
  loading.value = true
  campaign.value = null
  error.value = null

  try {
    const result = await createCampaign(text, current.signal)
    if (controller !== current) return
    campaign.value = result
    // Kenar çubuğundaki "Son kampanyalar" yeni kampanyayı göstersin.
    notifyCampaignsChanged()
  } catch (err) {
    if (controller !== current || isAbortError(err)) return
    error.value =
      err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
  } finally {
    if (controller === current) loading.value = false
  }
}

function retry() {
  void submit(lastSubmitted)
}

onBeforeUnmount(() => controller?.abort())

// Kampanya kaydedilince kitle adımı biter ve içerik adımı sürer.
const steps = computed(() =>
  stepStatuses({ status: campaign.value?.status ?? null, contentReady: false }),
)
// "İleri": kitle hesaplanıp taslak kaydedilince içerik ekranına geçilir. İlk aşama olduğu
// için "Geri" yoktur.
const nextStep = computed<NavStep>(() =>
  campaign.value
    ? { label: 'İçeriği hazırla', to: { name: 'campaign', params: { id: campaign.value.id } } }
    : { label: 'İçeriği hazırla', hint: 'Önce kitleyi hesaplayın.' },
)
const title = computed(() => (error.value ? errorTitle(error.value) : ''))
const reasons = computed(() =>
  error.value?.code === 'SEGMENT_INVALID' ? error.value.reasons.map(reasonMessage) : [],
)
</script>

<template>
  <main class="mx-auto grid w-full max-w-3xl gap-6">
    <PageHeader
      title="Kampanya önizleme"
      description="Brief'i yazın, kitleyi ve kanal sayılarını görün."
    >
      <template #badge>
        <StatusBadge v-if="campaign" :status="campaign.status" data-testid="campaign-state" />
      </template>
      <CampaignSteps :statuses="steps" />
    </PageHeader>

    <Card>
      <CardContent>
        <BriefForm v-model="brief" :loading="loading" @submit="submit" />
      </CardContent>
    </Card>

    <div class="grid gap-6" aria-live="polite">
      <div
        v-if="loading"
        class="text-muted-foreground flex items-center gap-2 text-sm"
        role="status"
      >
        <!-- Spinner svg'si kendi `status` rolünü taşır; metin zaten bu bölgede okunur. -->
        <Spinner class="motion-reduce:animate-none" role="presentation" aria-hidden="true" />
        <span>Kitle hesaplanıyor…</span>
      </div>
      <ErrorPanel
        v-else-if="error"
        :title="title"
        :reasons="reasons"
        :retryable="error.retryable"
        @retry="retry"
      />
      <template v-else-if="campaign">
        <AudienceCounts :audience="campaign.audience" />
        <SegmentSummary :segment="campaign.segment" />
        <AudienceMemberList :campaign-id="campaign.id" />
        <p class="text-muted-foreground text-sm wrap-anywhere">
          Taslak kampanya kaydedildi (kimlik: {{ campaign.id }}).
        </p>
      </template>
    </div>

    <StepNav :next="nextStep" />
  </main>
</template>
