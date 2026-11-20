<script setup lang="ts">
import { computed } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { ChevronLeftIcon, ChevronRightIcon, MoreHorizontalIcon } from '@lucide/vue'
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'

// Liste sayfalaması (shadcn `Pagination`): Önceki, sayfa numaraları (başta ve sonda sayfa, ortada
// üç nokta), Sonraki ve "11–20 / 191 kayıt · Sayfa 2 / 20" özeti. Tek sayfa varsa yalnızca özet
// görünür. Yüklenirken düğmeler kapanır. Dokunma hedefleri 40 px'tir.
const props = defineProps<{ page: number; total: number; limit: number; loading: boolean }>()
const emit = defineEmits<{ change: [page: number] }>()

// Dar ekranda (360 px) numaralar tek satıra sığsın diye etkin sayfanın yanındaki komşu sayfalar
// gösterilmez (1 … 5 … 20). matchMedia olmayan ortamda geniş düzen varsayılır.
const wide = useMediaQuery('(min-width: 640px)')
const siblingCount = computed(() =>
  typeof window !== 'undefined' && typeof window.matchMedia === 'function' && !wide.value ? 0 : 1,
)

const nf = new Intl.NumberFormat('tr-TR')
const pages = computed(() => Math.max(1, Math.ceil(props.total / props.limit)))
const from = computed(() => (props.page - 1) * props.limit + 1)
const to = computed(() => Math.min(props.page * props.limit, props.total))
const hasRows = computed(() => props.total > 0 && from.value <= props.total)

const itemClass =
  'transition-colors duration-200 data-[selected]:border-primary data-[selected]:bg-primary data-[selected]:text-primary-foreground data-[selected]:shadow-sm data-[selected]:hover:bg-primary/90'
</script>

<template>
  <nav
    aria-label="Sayfalama"
    class="flex flex-col items-center justify-between gap-3 border-t pt-4 sm:flex-row"
  >
    <p class="text-muted-foreground text-sm tabular-nums" aria-live="polite">
      <template v-if="hasRows">
        {{ nf.format(from) }}–{{ nf.format(to) }} / {{ nf.format(total) }} kayıt · Sayfa
        {{ nf.format(page) }} / {{ nf.format(pages) }}
      </template>
      <template v-else-if="total === 0">Kayıt yok</template>
      <template v-else>Sayfa {{ nf.format(page) }} / {{ nf.format(pages) }}</template>
    </p>

    <Pagination
      v-if="pages > 1"
      :page="page"
      :total="total"
      :items-per-page="limit"
      :sibling-count="siblingCount"
      show-edges
      :disabled="loading"
      class="mx-0 w-auto"
      @update:page="emit('change', $event)"
    >
      <PaginationContent v-slot="{ items }" class="flex-wrap justify-center">
        <PaginationPrevious size="lg" aria-label="Önceki sayfa">
          <ChevronLeftIcon data-icon="inline-start" aria-hidden="true" />
          <span class="hidden sm:inline">Önceki</span>
        </PaginationPrevious>

        <template v-for="(item, index) in items" :key="index">
          <PaginationItem
            v-if="item.type === 'page'"
            :value="item.value"
            :is-active="item.value === page"
            size="icon-lg"
            :class="itemClass"
            :aria-label="`Sayfa ${item.value}`"
            :aria-current="item.value === page ? 'page' : undefined"
          >
            {{ item.value }}
          </PaginationItem>
          <PaginationEllipsis v-else :index="index" class="size-10">
            <MoreHorizontalIcon aria-hidden="true" />
            <span class="sr-only">Diğer sayfalar</span>
          </PaginationEllipsis>
        </template>

        <PaginationNext size="lg" aria-label="Sonraki sayfa">
          <span class="hidden sm:inline">Sonraki</span>
          <ChevronRightIcon data-icon="inline-end" aria-hidden="true" />
        </PaginationNext>
      </PaginationContent>
    </Pagination>
  </nav>
</template>
