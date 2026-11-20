<script setup lang="ts">
import { ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { CirclePlusIcon, MegaphoneIcon } from '@lucide/vue'
import { getCampaignPage } from '@/api/workspace'
import { useResource } from '@/composables/useResource'
import { campaignTarget, campaignTargetLabel } from '@/campaign/links'
import { formatScheduledTime, deviceTimeZone } from '@/campaign/schedule'
import PageHeader from '@/components/PageHeader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import FilterSelect from '@/components/FilterSelect.vue'
import ListPagination from '@/components/ListPagination.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
const route = useRoute()
const statuses = ['', 'draft', 'approved', 'sending', 'completed']
/** Durum süzgecinin seçenekleri ("Tüm durumlar" FilterSelect'in kendi ilk seçeneğidir). */
const STATUS_OPTIONS = [
  { value: 'draft', label: 'Taslak' },
  { value: 'approved', label: 'Onaylandı, kuyrukta' },
  { value: 'sending', label: 'Gönderiliyor' },
  { value: 'completed', label: 'Tamamlandı' },
]
const state = ref(
  typeof route.query.status === 'string' && statuses.includes(route.query.status)
    ? route.query.status
    : '',
)
const page = ref(1)
const { data, loading, error, reload } = useResource((signal) =>
  getCampaignPage(page.value, state.value, signal),
)
watch([page, state], reload, { immediate: true })
watch(state, () => {
  page.value = 1
})
watch(
  () => route.query.status,
  (value) => {
    state.value = typeof value === 'string' && statuses.includes(value) ? value : ''
    page.value = 1
  },
)
const formatDate = (iso: string) => new Date(iso).toLocaleString('tr-TR')
</script>
<template>
  <main class="grid gap-8">
    <PageHeader
      :icon="MegaphoneIcon"
      title="Kampanyalar"
      description="Tüm kampanyalarınızı inceleyin; taslağa veya gönderim takibine devam edin."
    />
    <div class="flex flex-wrap items-end justify-between gap-4">
      <div class="grid w-full gap-1.5 sm:w-64">
        <Label for="campaign-filter">Durum</Label>
        <FilterSelect
          id="campaign-filter"
          v-model="state"
          :options="STATUS_OPTIONS"
          all-label="Tüm durumlar"
          data-testid="campaign-status-filter"
        />
      </div>
      <Button as-child>
        <RouterLink to="/">
          <CirclePlusIcon aria-hidden="true" />
          Yeni kampanya
        </RouterLink>
      </Button>
    </div>
    <Card
      ><CardContent class="grid gap-4">
        <div v-if="loading" role="status" class="grid gap-3">
          <span class="sr-only">Kampanyalar yükleniyor…</span><Skeleton class="h-20" /><Skeleton
            class="h-20"
          />
        </div>
        <ErrorPanel
          v-else-if="error"
          :title="error"
          :reasons="[]"
          :retryable="true"
          @retry="reload"
        />
        <p v-else-if="!data?.items.length" class="text-muted-foreground py-8 text-sm" role="status">
          Bu filtrede kampanya bulunamadı.
        </p>
        <ul v-else class="divide-y" data-testid="campaign-page-list">
          <li v-for="item in data.items" :key="item.id">
            <RouterLink
              :to="campaignTarget(item.id, item.status)"
              class="hover:bg-accent focus-visible:ring-ring grid min-w-0 gap-3 rounded-lg p-4 outline-none focus-visible:ring-2"
              data-testid="campaign-page-row"
              ><p class="line-clamp-2 font-medium wrap-anywhere">{{ item.brief }}</p>
              <div class="text-muted-foreground flex flex-wrap items-center gap-3 text-xs">
                <StatusBadge :status="item.status" :scheduled="Boolean(item.scheduled_at)" /><time
                  :datetime="item.created_at"
                  >{{ formatDate(item.created_at) }}</time
                ><span v-if="item.scheduled_at"
                  >Planlanan başlangıç: {{ formatScheduledTime(item.scheduled_at) }} ·
                  {{ deviceTimeZone() }}</span
                ><span class="text-primary">{{ campaignTargetLabel(item.status) }} →</span>
              </div></RouterLink
            >
          </li>
        </ul>
        <ListPagination
          v-if="data"
          :page="page"
          :total="data.total"
          :limit="data.limit"
          :loading="loading"
          @change="page = $event"
        /> </CardContent
    ></Card>
  </main>
</template>
