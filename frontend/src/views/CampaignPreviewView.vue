<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { ApiError, createCampaign, isAbortError, type Campaign } from '@/api/campaigns'
import { errorTitle } from '@/api/messages'
import { reasonMessage } from '@/segment/reasons'
import AudienceCounts from '@/components/AudienceCounts.vue'
import BriefForm from '@/components/BriefForm.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import SegmentSummary from '@/components/SegmentSummary.vue'

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

const title = computed(() => (error.value ? errorTitle(error.value) : ''))
const reasons = computed(() =>
  error.value?.code === 'SEGMENT_INVALID' ? error.value.reasons.map(reasonMessage) : [],
)
</script>

<template>
  <main>
    <h1>Kampanya önizleme</h1>
    <BriefForm v-model="brief" :loading="loading" @submit="submit" />

    <div class="result" aria-live="polite">
      <p v-if="loading" class="status" role="status">Kitle hesaplanıyor…</p>
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
        <p class="draft">Taslak kampanya kaydedildi (kimlik: {{ campaign.id }}).</p>
      </template>
    </div>
  </main>
</template>

<style scoped>
main {
  display: grid;
  gap: 1.5rem;
}
h1 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 700;
}
.result {
  display: grid;
  gap: 1.25rem;
}
.status {
  margin: 0;
  color: var(--app-muted);
}
.draft {
  margin: 0;
  color: var(--app-muted);
  font-size: 0.85rem;
}
</style>
