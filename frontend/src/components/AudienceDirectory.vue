<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FilterXIcon, SearchIcon, UsersRoundIcon, XIcon } from '@lucide/vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import type { AudiencePage, ChannelStatus } from '@/api/audience'
import {
  MAX_SEARCH_LENGTH,
  activeFilterCount,
  listCities,
  listDirectory,
  type DirectoryFilters,
  type StatusFilter,
} from '@/api/directory'
import { errorTitle } from '@/api/messages'
import { channelStatusLabel, totalPages } from '@/audience/members'
import ErrorPanel from '@/components/ErrorPanel.vue'
import FilterSelect from '@/components/FilterSelect.vue'
import ListPagination from '@/components/ListPagination.vue'
import MemberTable from '@/components/MemberTable.vue'
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
import { Skeleton } from '@/components/ui/skeleton'

// Kitleler sayfası: kampanyadan bağımsız kişi dizini. Arama (ad, e-posta, telefon), şehir, SMS ve
// e-posta uygunluğu süzgeçleri AND ile birleşir; her süzgeç değişikliği ilk sayfaya döner. Arama
// yazarken kısa bir beklemeyle (300 ms) kendiliğinden çalışır; Enter veya "Ara" beklemeden çalıştırır.
const SEARCH_DELAY_MS = 300
const nf = new Intl.NumberFormat('tr-TR')

const searchText = ref('')
const search = ref('') // listeyi gerçekten süzen, kırpılmış arama
const city = ref('')
const sms = ref('')
const email = ref('')

const page = ref(1)
const loading = ref(true)
const error = ref<ApiError | null>(null)
const data = ref<AudiencePage | null>(null)
const cities = ref<string[]>([])
const citiesFailed = ref(false)

let controller: AbortController | null = null
let citiesController: AbortController | null = null
let searchTimer: ReturnType<typeof setTimeout> | null = null
/** Hata sonrası "Yeniden dene" için son istenen sayfa. */
let requested = 1

/** Sunucunun kabul ettiği durumlar dışındaki her değer "süzme yok" sayılır. */
function toStatus(value: string): StatusFilter {
  return value === 'eligible' || value === 'no_consent' || value === 'no_address' ? value : ''
}

const filters = computed<DirectoryFilters>(() => ({
  search: search.value,
  city: city.value,
  sms: toStatus(sms.value),
  email: toStatus(email.value),
}))
const activeCount = computed(() => activeFilterCount(filters.value))

async function load(target: number) {
  // Önceki istek sürüyorsa iptal et; geç gelen yanıt sonucu ezmemeli.
  controller?.abort()
  const current = new AbortController()
  controller = current
  requested = target
  loading.value = true
  error.value = null
  try {
    const result = await listDirectory(filters.value, target, current.signal)
    if (controller !== current) return
    const last = totalPages(result.total, result.limit)
    if (result.items.length === 0 && result.total > 0 && target > last) {
      // Süzgeç sonucu kısalttı: boş bir sayfa göstermek yerine son sayfaya git.
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

async function loadCities() {
  citiesController?.abort()
  const current = new AbortController()
  citiesController = current
  try {
    const result = await listCities(current.signal)
    if (citiesController !== current) return
    cities.value = result
    citiesFailed.value = false
  } catch (err) {
    if (citiesController !== current || isAbortError(err)) return
    citiesFailed.value = true
  }
}

function clearTimer() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = null
}

/** Arama kutusundaki metni süzgece uygular (kırpılmış). Metin değişmediyse istek yapılmaz. */
function applySearch() {
  clearTimer()
  search.value = searchText.value.trim()
}

function onSearchInput() {
  clearTimer()
  searchTimer = setTimeout(applySearch, SEARCH_DELAY_MS)
}

function clearSearch() {
  searchText.value = ''
  applySearch()
}

function clearFilters() {
  clearTimer()
  searchText.value = ''
  search.value = ''
  city.value = ''
  sms.value = ''
  email.value = ''
}

// Süzgeç değişince ilk sayfa baştan okunur (aynı değere dönülmesi yeni istek sayılmaz).
watch(filters, () => void load(1))

onMounted(() => {
  void load(1)
  void loadCities()
})
onBeforeUnmount(() => {
  clearTimer()
  controller?.abort()
  citiesController?.abort()
})

const lastPage = computed(() => (data.value ? totalPages(data.value.total, data.value.limit) : 1))

function goTo(target: number) {
  if (loading.value || target < 1 || target > lastPage.value || target === page.value) return
  void load(target)
}

const cityOptions = computed(() => cities.value.map((c) => ({ value: c, label: c })))

const STATUS_OPTIONS: { value: ChannelStatus; label: string }[] = (
  ['eligible', 'no_consent', 'no_address'] as const
).map((value) => ({ value, label: channelStatusLabel(value) }))
</script>

<template>
  <section aria-label="Kişi dizini" class="grid min-w-0 gap-5" data-testid="audience-directory">
    <form
      role="search"
      aria-label="Kişi süzgeçleri"
      class="grid gap-4"
      data-testid="directory-filters"
      @submit.prevent="applySearch"
    >
      <div class="grid gap-1.5">
        <Label for="directory-search">Kişi ara</Label>
        <InputGroup class="h-10">
          <InputGroupAddon>
            <SearchIcon aria-hidden="true" />
          </InputGroupAddon>
          <InputGroupInput
            id="directory-search"
            v-model="searchText"
            type="search"
            autocomplete="off"
            :maxlength="MAX_SEARCH_LENGTH"
            placeholder="Ad, e-posta veya telefon"
            class="[&::-webkit-search-cancel-button]:hidden"
            data-testid="directory-search"
            @input="onSearchInput"
          />
          <InputGroupAddon align="inline-end" class="gap-1">
            <InputGroupButton
              v-if="searchText"
              size="icon-xs"
              aria-label="Aramayı temizle"
              data-testid="directory-search-clear"
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

      <div class="grid gap-3 sm:grid-cols-3">
        <div class="grid gap-1.5">
          <Label for="directory-city">Şehir</Label>
          <FilterSelect
            id="directory-city"
            v-model="city"
            :options="cityOptions"
            :all-label="citiesFailed ? 'Şehirler yüklenemedi' : 'Tüm şehirler'"
            :disabled="citiesFailed"
            data-testid="directory-city"
          />
        </div>
        <div class="grid gap-1.5">
          <Label for="directory-sms">SMS uygunluğu</Label>
          <FilterSelect
            id="directory-sms"
            v-model="sms"
            :options="STATUS_OPTIONS"
            all-label="Tümü"
            data-testid="directory-sms"
          />
        </div>
        <div class="grid gap-1.5">
          <Label for="directory-email">E-posta uygunluğu</Label>
          <FilterSelect
            id="directory-email"
            v-model="email"
            :options="STATUS_OPTIONS"
            all-label="Tümü"
            data-testid="directory-email"
          />
        </div>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="text-muted-foreground text-xs">
          Demo verisidir. Kanal izni olmayan kişiler de listelenir; ilgili kanal için "uygun değil"
          görünür.
        </p>
        <Button
          v-if="activeCount > 0"
          type="button"
          variant="ghost"
          size="sm"
          data-testid="directory-clear"
          @click="clearFilters"
        >
          <FilterXIcon aria-hidden="true" />
          Filtreleri temizle
          <Badge variant="secondary" class="ml-0.5">{{ activeCount }}</Badge>
        </Button>
      </div>
    </form>

    <div class="grid gap-4">
      <!-- İlk yüklemede iskelet. Sonraki isteklerde eski satırlar görünür kalır (aria-busy). -->
      <div v-if="loading && !data" class="grid gap-2" role="status" data-testid="directory-loading">
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

      <Empty v-else-if="data && data.total === 0" class="border" data-testid="directory-empty">
        <EmptyHeader>
          <EmptyMedia variant="icon"><UsersRoundIcon aria-hidden="true" /></EmptyMedia>
          <EmptyTitle>
            {{ activeCount > 0 ? 'Filtrelere uyan kişi yok' : 'Henüz kişi yok' }}
          </EmptyTitle>
          <EmptyDescription v-if="activeCount > 0">
            Aramayı veya filtreleri değiştirmeyi deneyin.
          </EmptyDescription>
        </EmptyHeader>
        <Button v-if="activeCount > 0" type="button" variant="outline" @click="clearFilters">
          Filtreleri temizle
        </Button>
      </Empty>

      <template v-else-if="data">
        <p class="text-sm font-medium" aria-live="polite" data-testid="directory-total">
          {{ nf.format(data.total) }} kişi
        </p>
        <div
          :aria-busy="loading"
          :class="{ 'opacity-60': loading }"
          class="transition-opacity duration-200"
          data-testid="directory-page"
        >
          <MemberTable
            :members="data.items"
            :caption="`Kişi dizini, kimliğe göre sıralı; sayfa ${page} / ${lastPage}`"
          />
        </div>
        <ListPagination
          :page="page"
          :total="data.total"
          :limit="data.limit"
          :loading="loading"
          @change="goTo"
        />
      </template>
    </div>
  </section>
</template>
