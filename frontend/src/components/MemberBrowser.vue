<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { getMembers, type Member, type ChannelStatus } from '@/api/workspace'
import ErrorPanel from '@/components/ErrorPanel.vue'
import ListPagination from '@/components/ListPagination.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'

const props = defineProps<{ campaignId?: string }>()
const page = ref(1)
const total = ref(0)
const rows = ref<Member[]>([])
const loading = ref(true)
const error = ref('')
const search = ref('')
const submittedSearch = ref('')
let controller: AbortController | null = null

async function load() {
  controller?.abort()
  const current = new AbortController()
  controller = current
  loading.value = true
  error.value = ''
  try {
    const data = await getMembers(
      page.value,
      submittedSearch.value,
      props.campaignId,
      current.signal,
    )
    if (controller !== current) return
    rows.value = data.items
    total.value = data.total
  } catch (err) {
    if (controller === current && !isAbortError(err))
      error.value = err instanceof ApiError ? err.message : 'Kişiler yüklenemedi.'
  } finally {
    if (controller === current) loading.value = false
  }
}
function submit() {
  if (submittedSearch.value === search.value.trim() && page.value === 1) {
    void load()
    return
  }
  page.value = 1
  submittedSearch.value = search.value.trim()
}
function statusLabel(status: ChannelStatus, channel: string) {
  return status === 'eligible' ? 'Uygun' : status === 'no_consent' ? 'İzin yok' : `${channel} yok`
}
watch([page, submittedSearch], load, { immediate: true })
watch(
  () => props.campaignId,
  () => {
    if (page.value !== 1) page.value = 1
    else void load()
  },
)
onBeforeUnmount(() => {
  controller?.abort()
  controller = null
})
</script>
<template>
  <section
    class="grid min-w-0 gap-4"
    :aria-label="campaignId ? 'Segmente uyan kişiler' : 'Demo kullanıcıları'"
  >
    <form
      v-if="!campaignId"
      class="flex flex-col gap-2 sm:flex-row sm:items-end"
      @submit.prevent="submit"
    >
      <div class="grid flex-1 gap-2">
        <Label for="member-search">Kişi ara</Label
        ><Input
          id="member-search"
          v-model="search"
          maxlength="100"
          placeholder="Ad, şehir, e-posta veya telefon"
        />
      </div>
      <Button type="submit" :disabled="loading">Ara</Button>
    </form>
    <p class="text-muted-foreground text-sm">
      Demo verisidir. Kesin alıcılar onay sırasında yeniden belirlenir; kanal izni olmayan kişiler
      de listelenir.
    </p>
    <div v-if="loading" role="status" class="grid gap-3">
      <span class="sr-only">Kişiler yükleniyor…</span><Skeleton class="h-24" /><Skeleton
        class="h-24"
      />
    </div>
    <ErrorPanel v-else-if="error" :title="error" :reasons="[]" :retryable="true" @retry="load" />
    <p v-else-if="rows.length === 0" class="text-muted-foreground py-6 text-sm" role="status">
      Bu sayfada kişi bulunamadı.
    </p>
    <ul v-else class="grid gap-3" data-testid="member-list">
      <li
        v-for="member in rows"
        :key="member.id"
        class="min-w-0 rounded-lg border p-4"
        data-testid="member-row"
      >
        <dl class="grid min-w-0 gap-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <dt class="text-muted-foreground text-xs">Ad</dt>
            <dd class="mt-1 font-medium wrap-anywhere">
              {{ member.first_name ?? 'Ad bilgisi yok' }}
            </dd>
          </div>
          <div>
            <dt class="text-muted-foreground text-xs">Şehir</dt>
            <dd class="mt-1 wrap-anywhere">{{ member.city }}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground text-xs">E-posta</dt>
            <dd class="mt-1 wrap-anywhere">{{ member.email ?? 'E-posta yok' }}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground text-xs">Telefon</dt>
            <dd class="mt-1 wrap-anywhere">{{ member.phone ?? 'Telefon yok' }}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground text-xs">SMS uygunluğu</dt>
            <dd class="mt-1">
              <Badge :variant="member.sms_status === 'eligible' ? 'secondary' : 'outline'">{{
                statusLabel(member.sms_status, 'Telefon')
              }}</Badge>
            </dd>
          </div>
          <div>
            <dt class="text-muted-foreground text-xs">E-posta uygunluğu</dt>
            <dd class="mt-1">
              <Badge :variant="member.email_status === 'eligible' ? 'secondary' : 'outline'">{{
                statusLabel(member.email_status, 'E-posta')
              }}</Badge>
            </dd>
          </div>
        </dl>
      </li>
    </ul>
    <ListPagination
      :page="page"
      :total="total"
      :limit="10"
      :loading="loading"
      @change="page = $event"
    />
  </section>
</template>
