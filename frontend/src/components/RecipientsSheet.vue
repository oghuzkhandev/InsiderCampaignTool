<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  ActivityIcon,
  FilterXIcon,
  MailIcon,
  MessageSquareIcon,
  SearchIcon,
  UserRoundIcon,
  UsersRoundIcon,
  XIcon,
} from '@lucide/vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import {
  MAX_RECIPIENT_SEARCH_LENGTH,
  listRecipients,
  type ChannelFilter,
  type RecipientFilters,
  type RecipientPage,
  type StatusFilter,
} from '@/api/recipients'
import { errorTitle } from '@/api/messages'
import { totalPages } from '@/audience/members'
import {
  CHANNEL_LABELS,
  CHANNEL_OPTIONS,
  MESSAGE_STATUS_OPTIONS,
  failureReasonLabel,
  formatSentAt,
  recipientInitial,
} from '@/delivery/recipients'
import ErrorPanel from '@/components/ErrorPanel.vue'
import FilterSelect from '@/components/FilterSelect.vue'
import ListPagination from '@/components/ListPagination.vue'
import MessageStatusBadge from '@/components/MessageStatusBadge.vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

// "Alıcılar" çekmecesi: kampanyada mesaj alan (veya alacak) herkes; kim, hangi kanaldan, hangi
// adrese ve ne oldu. Ad ve adres onay anındaki kopyadır. Arama (ad, e-posta, telefon), kanal ve durum
// süzgeçleri AND ile birleşir; her süzgeç değişikliği ilk sayfaya döner. Arama yazarken kısa bir
// beklemeyle (300 ms) kendiliğinden çalışır. Çekmece açıkken gönderim sürüyorsa (`live`) liste
// birkaç saniyede bir sessizce tazelenir: durumlar canlı değişir, satırlar titremez.
const props = defineProps<{ campaignId: string; live: boolean }>()
const open = defineModel<boolean>('open', { required: true })

const SEARCH_DELAY_MS = 300
const REFRESH_MS = 5_000
const nf = new Intl.NumberFormat('tr-TR')

const searchText = ref('')
const search = ref('') // listeyi gerçekten süzen, kırpılmış arama
const channel = ref('')
const status = ref('')

const page = ref(1)
const loading = ref(false)
const error = ref<ApiError | null>(null)
const data = ref<RecipientPage | null>(null)

let controller: AbortController | null = null
let searchTimer: ReturnType<typeof setTimeout> | null = null
let refreshTimer: ReturnType<typeof setInterval> | null = null
/** Hata sonrası "Yeniden dene" için son istenen sayfa. */
let requested = 1

/** Sunucunun kabul ettiği değerler dışındaki her şey "süzme yok" sayılır. */
function toChannel(value: string): ChannelFilter {
  return value === 'sms' || value === 'email' ? value : ''
}
function toStatus(value: string): StatusFilter {
  const match = MESSAGE_STATUS_OPTIONS.find((o) => o.value === value)
  return match ? match.value : ''
}

const filters = computed<RecipientFilters>(() => ({
  search: search.value,
  channel: toChannel(channel.value),
  status: toStatus(status.value),
}))
const activeCount = computed(
  () => [filters.value.search, filters.value.channel, filters.value.status].filter(Boolean).length,
)

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
    const result = await listRecipients(props.campaignId, filters.value, target, current.signal)
    if (controller !== current) return
    const last = totalPages(result.total, result.limit)
    if (result.items.length === 0 && result.total > 0 && target > last) {
      // Süzgeç veya liste kısaldı: boş bir sayfa göstermek yerine son sayfaya git.
      await load(last, silent)
      return
    }
    data.value = result
    page.value = target
    error.value = null
  } catch (err) {
    if (controller !== current || isAbortError(err)) return
    // Sessiz tazeleme başarısız olursa ekrandaki liste kalır; bir sonraki tur yeniden dener.
    if (silent) return
    error.value =
      err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
  } finally {
    if (controller === current) loading.value = false
  }
}

function clearSearchTimer() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = null
}
function stopRefresh() {
  if (refreshTimer) clearInterval(refreshTimer)
  refreshTimer = null
}
function syncRefresh() {
  stopRefresh()
  if (open.value && props.live) {
    refreshTimer = setInterval(() => {
      if (!loading.value) void load(page.value, true)
    }, REFRESH_MS)
  }
}

/** Arama kutusundaki metni süzgece uygular (kırpılmış). Metin değişmediyse istek yapılmaz. */
function applySearch() {
  clearSearchTimer()
  search.value = searchText.value.trim()
}
function onSearchInput() {
  clearSearchTimer()
  searchTimer = setTimeout(applySearch, SEARCH_DELAY_MS)
}
function clearSearch() {
  searchText.value = ''
  applySearch()
}
function clearFilters() {
  clearSearchTimer()
  searchText.value = ''
  search.value = ''
  channel.value = ''
  status.value = ''
}

function reset() {
  controller?.abort()
  clearSearchTimer()
  data.value = null
  error.value = null
  page.value = 1
  clearFilters()
}

// Süzgeç değişince (çekmece açıkken) ilk sayfa baştan okunur.
watch(filters, () => {
  if (open.value) void load(1)
})
// Çekmece her açılışta güncel listeyi okur; kapanınca bekleyen istek ve zamanlayıcılar bırakılır.
// `immediate`: çekmece açık olarak çizilirse de liste okunur.
watch(
  open,
  (isOpen) => {
    if (isOpen) void load(1)
    else {
      controller?.abort()
      clearSearchTimer()
    }
    syncRefresh()
  },
  { immediate: true },
)
watch(() => props.live, syncRefresh)
// Başka kampanyaya geçilirse eski kampanyanın satırları ve süzgeçleri kalmaz.
watch(
  () => props.campaignId,
  () => {
    // Etkin süzgeç varsa onları sıfırlamak zaten ilk sayfayı yeniden okutur (tek istek yeter).
    const refetchedByFilters = activeCount.value > 0
    reset()
    if (open.value && !refetchedByFilters) void load(1)
  },
)
onBeforeUnmount(() => {
  controller?.abort()
  clearSearchTimer()
  stopRefresh()
})

const lastPage = computed(() => (data.value ? totalPages(data.value.total, data.value.limit) : 1))

function goTo(target: number) {
  if (loading.value || target < 1 || target > lastPage.value || target === page.value) return
  void load(target)
}

const columns = [
  { id: 'recipient', label: 'Alıcı', icon: UserRoundIcon },
  { id: 'channel', label: 'Kanal', icon: MessageSquareIcon },
  { id: 'status', label: 'Durum', icon: ActivityIcon },
] as const
const channelIcon = { sms: MessageSquareIcon, email: MailIcon } as const
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent
      side="right"
      :show-close-button="false"
      class="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-3xl"
      data-testid="recipients-sheet"
    >
      <SheetHeader class="gap-3 p-5 pb-4">
        <div class="flex items-start gap-3.5">
          <span
            class="border-primary/20 bg-primary/10 text-primary grid size-11 shrink-0 place-items-center rounded-xl border"
            aria-hidden="true"
          >
            <UsersRoundIcon class="size-5" :stroke-width="1.75" />
          </span>
          <div class="grid min-w-0 flex-1 gap-1">
            <SheetTitle class="text-lg font-semibold tracking-tight">Alıcılar</SheetTitle>
            <SheetDescription>
              Bu kampanyada mesaj alan kişiler. Ad ve adres, kampanya onaylandığı andaki halidir.
            </SheetDescription>
          </div>
          <SheetClose as-child>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Alıcılar çekmecesini kapat"
              data-testid="recipients-close"
            >
              <XIcon aria-hidden="true" />
            </Button>
          </SheetClose>
        </div>
      </SheetHeader>
      <Separator />

      <form
        role="search"
        aria-label="Alıcı süzgeçleri"
        class="bg-muted/30 grid gap-3 border-b p-4 sm:px-5"
        data-testid="recipients-filters"
        @submit.prevent="applySearch"
      >
        <div class="grid gap-1.5">
          <Label for="recipients-search">Alıcı ara</Label>
          <InputGroup class="bg-background h-10">
            <InputGroupAddon>
              <SearchIcon aria-hidden="true" />
            </InputGroupAddon>
            <InputGroupInput
              id="recipients-search"
              v-model="searchText"
              type="search"
              autocomplete="off"
              :maxlength="MAX_RECIPIENT_SEARCH_LENGTH"
              placeholder="Ad, e-posta veya telefon"
              class="[&::-webkit-search-cancel-button]:hidden"
              data-testid="recipients-search"
              @input="onSearchInput"
            />
            <InputGroupAddon align="inline-end" class="gap-1">
              <InputGroupButton
                v-if="searchText"
                size="icon-xs"
                aria-label="Aramayı temizle"
                data-testid="recipients-search-clear"
                @click="clearSearch"
              >
                <XIcon aria-hidden="true" />
              </InputGroupButton>
              <InputGroupButton type="submit" variant="default" size="sm" class="h-8 px-3">
                Ara
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div class="grid gap-1.5">
            <Label for="recipients-channel">Kanal</Label>
            <FilterSelect
              id="recipients-channel"
              v-model="channel"
              :options="CHANNEL_OPTIONS"
              all-label="Tüm kanallar"
              data-testid="recipients-channel"
            />
          </div>
          <div class="grid gap-1.5">
            <Label for="recipients-status">Durum</Label>
            <FilterSelect
              id="recipients-status"
              v-model="status"
              :options="MESSAGE_STATUS_OPTIONS"
              all-label="Tüm durumlar"
              data-testid="recipients-status"
            />
          </div>
        </div>
      </form>

      <div class="min-h-0 flex-1 overflow-y-auto p-4 sm:px-5" data-testid="recipients-body">
        <!-- İlk yüklemede iskelet. Sonraki isteklerde eski satırlar görünür kalır (aria-busy). -->
        <div
          v-if="(loading && !data) || (!data && !error)"
          class="grid gap-2"
          role="status"
          data-testid="recipients-loading"
        >
          <span class="sr-only">Alıcılar yükleniyor…</span>
          <Skeleton v-for="n in 6" :key="n" class="h-14 w-full" />
        </div>

        <ErrorPanel
          v-else-if="error"
          :title="`Alıcılar yüklenemedi. ${errorTitle(error)}`"
          :reasons="[]"
          :retryable="error.retryable"
          @retry="load(requested)"
        />

        <Empty v-else-if="data && data.total === 0" class="border" data-testid="recipients-empty">
          <EmptyHeader>
            <EmptyMedia variant="icon"><UsersRoundIcon aria-hidden="true" /></EmptyMedia>
            <EmptyTitle>
              {{ activeCount > 0 ? 'Filtrelere uyan alıcı yok' : 'Henüz alıcı yok' }}
            </EmptyTitle>
            <EmptyDescription>
              {{
                activeCount > 0
                  ? 'Aramayı veya filtreleri değiştirmeyi deneyin.'
                  : 'Mesajlar kampanya onaylandığında alıcılarla birlikte oluşur.'
              }}
            </EmptyDescription>
          </EmptyHeader>
          <Button v-if="activeCount > 0" type="button" variant="outline" @click="clearFilters">
            Filtreleri temizle
          </Button>
        </Empty>

        <div v-else-if="data" class="grid gap-3">
          <div class="flex items-center justify-between gap-2">
            <p class="text-sm font-medium" aria-live="polite" data-testid="recipients-total">
              {{ nf.format(data.total) }} alıcı
            </p>
            <Button
              v-if="activeCount > 0"
              type="button"
              variant="ghost"
              size="sm"
              data-testid="recipients-clear"
              @click="clearFilters"
            >
              <FilterXIcon aria-hidden="true" />
              Filtreleri temizle
              <Badge variant="secondary" class="ml-0.5">{{ activeCount }}</Badge>
            </Button>
          </div>

          <div
            class="overflow-hidden rounded-xl border transition-opacity duration-200"
            :class="{ 'opacity-60': loading }"
            :aria-busy="loading"
            data-testid="recipients-page"
          >
            <Table>
              <TableCaption class="sr-only">
                Kampanya alıcıları, ada göre sıralı; sayfa {{ page }} / {{ lastPage }}
              </TableCaption>
              <TableHeader class="bg-muted/50">
                <TableRow class="hover:bg-transparent">
                  <TableHead
                    v-for="column in columns"
                    :key="column.id"
                    class="h-10 border-r px-3 text-xs font-semibold last:border-r-0"
                  >
                    <span class="inline-flex items-center gap-2">
                      <component
                        :is="column.icon"
                        class="text-muted-foreground size-4 shrink-0"
                        aria-hidden="true"
                      />
                      {{ column.label }}
                    </span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow
                  v-for="item in data.items"
                  :key="item.id"
                  class="last:border-b-0"
                  data-testid="recipient-row"
                >
                  <TableCell class="border-r px-3 py-2.5 align-middle whitespace-normal">
                    <div class="flex min-w-0 items-center gap-3">
                      <Avatar size="sm" aria-hidden="true">
                        <AvatarFallback class="bg-primary/10 text-primary text-xs font-semibold">
                          {{ recipientInitial(item.first_name, item.address) }}
                        </AvatarFallback>
                      </Avatar>
                      <div class="grid min-w-0 gap-0.5">
                        <span
                          v-if="item.first_name"
                          class="truncate font-medium"
                          data-testid="recipient-name"
                        >
                          {{ item.first_name }}
                        </span>
                        <span
                          v-else
                          class="text-muted-foreground truncate italic"
                          data-testid="recipient-name"
                          data-missing="true"
                        >
                          Ad yok
                        </span>
                        <span
                          class="text-muted-foreground text-xs wrap-anywhere"
                          data-testid="recipient-address"
                        >
                          {{ item.address }}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell
                    class="border-r px-3 py-2.5 align-middle"
                    data-testid="recipient-channel"
                  >
                    <span class="inline-flex items-center gap-1.5 text-sm">
                      <component
                        :is="channelIcon[item.channel]"
                        class="text-muted-foreground size-4 shrink-0"
                        aria-hidden="true"
                      />
                      {{ CHANNEL_LABELS[item.channel] }}
                    </span>
                  </TableCell>
                  <TableCell
                    class="px-3 py-2.5 align-middle whitespace-normal"
                    data-testid="recipient-status"
                  >
                    <div class="grid justify-items-start gap-1">
                      <MessageStatusBadge :status="item.status" :attempts="item.attempts" />
                      <span
                        v-if="formatSentAt(item.sent_at)"
                        class="text-muted-foreground text-xs tabular-nums"
                        data-testid="recipient-sent-at"
                      >
                        {{ formatSentAt(item.sent_at) }}
                      </span>
                      <span
                        v-if="item.status === 'failed' && failureReasonLabel(item.failure_reason)"
                        class="text-destructive text-xs"
                        data-testid="recipient-failure"
                      >
                        {{ failureReasonLabel(item.failure_reason) }}
                      </span>
                    </div>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>
      </div>

      <div v-if="data && data.total > 0" class="bg-background border-t px-4 py-3 sm:px-5">
        <ListPagination
          :page="page"
          :total="data.total"
          :limit="data.limit"
          :loading="loading"
          class="border-t-0 pt-0"
          @change="goTo"
        />
      </div>
    </SheetContent>
  </Sheet>
</template>
