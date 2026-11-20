<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { RouterLink } from 'vue-router'
import { ArrowLeftIcon, ArrowRightIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'

// Kampanya akışında bir önceki ve bir sonraki aşamaya geçiş: "Geri" ve "İleri". Hedef henüz
// açılamıyorsa (örneğin içerik onaylanmadan gönderim takibi) düğme kapalıdır ve nedeni
// yanında yazar; böylece akışın nereye gittiği görünür ama atlanamaz.
export interface NavStep {
  /** Hedefin adı: "Geri: Kampanyaya dön", "İleri: Gönderimi izle". */
  label: string
  /**
   * Düğmede görünen tam metin; verilirse "Geri: ..." / "İleri: ..." kalıbı kullanılmaz. Gidilen
   * yer önceki aşama değilse (yeni kampanya başlatmak gibi) kullanıcı "geri döndüğünü"
   * sanmasın diye düğme kendi adıyla anılır.
   */
  text?: string
  /** Yoksa düğme kapalıdır. */
  to?: RouteLocationRaw
  /** Düğmenin altında görünen kısa not: kapalıysa nedeni, açıksa bilmesi gereken bir bilgi. */
  hint?: string
  /** Fareyle üzerine gelince görünen açıklama. */
  title?: string
  /** Bağlantının `data-testid` değeri. */
  testid?: string
}

withDefaults(
  defineProps<{
    back?: NavStep
    next?: NavStep
    /** true ise dar ekranda ekranın altına yapışır (ana eylemlere her zaman ulaşılsın). */
    sticky?: boolean
  }>(),
  { back: undefined, next: undefined, sticky: false },
)
</script>

<template>
  <nav
    aria-label="Adım gezintisi"
    data-testid="step-nav"
    class="grid gap-1"
    :class="
      sticky
        ? 'bg-background/95 sticky bottom-0 z-10 -mx-4 border-t px-4 py-3 backdrop-blur sm:static sm:mx-0 sm:border-0 sm:bg-transparent sm:p-0 sm:backdrop-blur-none'
        : ''
    "
  >
    <div class="flex items-center justify-between gap-3">
      <template v-if="back">
        <Button
          v-if="back.to"
          as-child
          variant="outline"
          size="lg"
          class="h-auto min-h-10 min-w-0 flex-1 py-2 text-center whitespace-normal sm:flex-none"
        >
          <RouterLink
            :to="back.to"
            :title="back.title"
            :aria-describedby="back.hint ? 'step-nav-back-hint' : undefined"
            :data-testid="back.testid ?? 'nav-back'"
          >
            <ArrowLeftIcon aria-hidden="true" />
            {{ back.text ?? `Geri: ${back.label}` }}
          </RouterLink>
        </Button>
        <Button
          v-else
          type="button"
          variant="outline"
          size="lg"
          class="h-auto min-h-10 min-w-0 flex-1 py-2 text-center whitespace-normal sm:flex-none"
          disabled
          data-testid="nav-back-disabled"
          aria-describedby="step-nav-back-hint"
        >
          <ArrowLeftIcon aria-hidden="true" />
          {{ back.text ?? `Geri: ${back.label}` }}
        </Button>
      </template>
      <span v-else aria-hidden="true" />

      <template v-if="next">
        <Button
          v-if="next.to"
          as-child
          size="lg"
          class="h-auto min-h-10 min-w-0 flex-1 py-2 text-center whitespace-normal sm:flex-none"
        >
          <RouterLink
            :to="next.to"
            :title="next.title"
            :aria-describedby="next.hint ? 'step-nav-next-hint' : undefined"
            :data-testid="next.testid ?? 'nav-next'"
          >
            {{ next.text ?? `İleri: ${next.label}` }}
            <ArrowRightIcon aria-hidden="true" />
          </RouterLink>
        </Button>
        <Button
          v-else
          type="button"
          size="lg"
          class="h-auto min-h-10 min-w-0 flex-1 py-2 text-center whitespace-normal sm:flex-none"
          disabled
          data-testid="nav-next-disabled"
          aria-describedby="step-nav-next-hint"
        >
          {{ next.text ?? `İleri: ${next.label}` }}
          <ArrowRightIcon aria-hidden="true" />
        </Button>
      </template>
    </div>

    <div class="flex justify-between gap-3 text-xs">
      <p v-if="back?.hint" id="step-nav-back-hint" class="text-muted-foreground">
        {{ back.hint }}
      </p>
      <span v-else aria-hidden="true" />
      <p v-if="next?.hint" id="step-nav-next-hint" class="text-muted-foreground text-right">
        {{ next.hint }}
      </p>
    </div>
  </nav>
</template>
