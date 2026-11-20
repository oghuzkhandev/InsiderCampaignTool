<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { CircleCheckIcon, InfoIcon, OctagonXIcon, TriangleAlertIcon, XIcon } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { NOTIFY_TICK_MS, type NotifyKind } from './types'

// Bildirim kartı (vue-sonner'ın `toast.custom`'ı ile gösterilir): türüne göre ikonlu karo, başlık,
// isteğe bağlı açıklama, kalan süreyi gösteren sayaç ve alttaki azalan çubuk, kapatma düğmesi.
// Ömrünü bu bileşen yönetir (sonner'a süre verilmez): süre dolunca kendini kapatır. İmleç veya
// odak kartın üzerindeyken sayaç duraklar (okurken kapanmasın). Metinler HTML olarak yorumlanmaz.
const props = defineProps<{
  id: string | number
  kind: NotifyKind
  title: string
  description?: string
  /** Kartın ekranda kalacağı toplam süre (ms). */
  durationMs: number
}>()

const remaining = ref(props.durationMs)
const paused = ref(false)
let timer: ReturnType<typeof setInterval> | null = null
let last = Date.now()

function stop() {
  if (timer) clearInterval(timer)
  timer = null
}

function close() {
  stop()
  toast.dismiss(props.id)
}

function tick() {
  const now = Date.now()
  const elapsed = now - last
  last = now
  if (paused.value) return // duraklıyken süre akmaz
  remaining.value = Math.max(0, remaining.value - elapsed)
  if (remaining.value === 0) close()
}

function pause() {
  paused.value = true
}
function resume() {
  last = Date.now() // duraklama süresi sayılmaz
  paused.value = false
}

onMounted(() => {
  last = Date.now()
  timer = setInterval(tick, NOTIFY_TICK_MS)
})
onBeforeUnmount(stop)

const seconds = computed(() => Math.ceil(remaining.value / 1000))
const percent = computed(() =>
  props.durationMs > 0 ? Math.round((remaining.value / props.durationMs) * 1000) / 10 : 0,
)

const looks = {
  success: {
    icon: CircleCheckIcon,
    tile: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-400/15 dark:text-emerald-300',
    bar: 'bg-emerald-500 dark:bg-emerald-400',
  },
  warning: {
    icon: TriangleAlertIcon,
    tile: 'bg-amber-100 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300',
    bar: 'bg-amber-500 dark:bg-amber-400',
  },
  error: {
    icon: OctagonXIcon,
    tile: 'bg-red-100 text-red-700 dark:bg-red-400/15 dark:text-red-300',
    bar: 'bg-red-500 dark:bg-red-400',
  },
  info: {
    icon: InfoIcon,
    tile: 'bg-sky-100 text-sky-700 dark:bg-sky-400/15 dark:text-sky-300',
    bar: 'bg-sky-500 dark:bg-sky-400',
  },
} as const
const look = computed(() => looks[props.kind])
// Hata ve uyarı hemen okunur (alert), başarı ve bilgi nazikçe (status).
const role = computed(() =>
  props.kind === 'error' || props.kind === 'warning' ? 'alert' : 'status',
)
</script>

<template>
  <div
    :role="role"
    class="bg-popover text-popover-foreground w-[min(calc(100vw-2rem),26rem)] overflow-hidden rounded-xl border shadow-lg"
    :data-kind="kind"
    :data-paused="paused"
    data-testid="notification"
    @mouseenter="pause"
    @mouseleave="resume"
    @focusin="pause"
    @focusout="resume"
  >
    <div class="flex items-start gap-3 p-3.5">
      <span
        class="grid size-9 shrink-0 place-items-center rounded-lg"
        :class="look.tile"
        aria-hidden="true"
      >
        <component :is="look.icon" class="size-5" :stroke-width="1.75" />
      </span>

      <div class="grid min-w-0 flex-1 gap-0.5 pt-0.5">
        <p class="text-sm font-semibold wrap-anywhere" data-testid="notification-title">
          {{ title }}
        </p>
        <p
          v-if="description"
          class="text-muted-foreground text-sm wrap-anywhere"
          data-testid="notification-description"
        >
          {{ description }}
        </p>
      </div>

      <div class="flex shrink-0 items-center gap-1">
        <!-- Kalan süre: her saniye ekran okuyucuya okunmaması için gizli; çubuk ve sayı görsel göstergedir. -->
        <span
          class="text-muted-foreground text-xs tabular-nums"
          aria-hidden="true"
          data-testid="notification-time"
        >
          {{ paused ? 'Duraklatıldı' : `${seconds} sn` }}
        </span>
        <button
          type="button"
          class="text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring/50 grid size-7 place-items-center rounded-md transition-colors duration-200 outline-none focus-visible:ring-2"
          aria-label="Bildirimi kapat"
          data-testid="notification-close"
          @click="close"
        >
          <XIcon class="size-4" aria-hidden="true" />
        </button>
      </div>
    </div>

    <div class="bg-muted h-1" aria-hidden="true">
      <div
        class="h-full transition-[width] duration-100 ease-linear"
        :class="look.bar"
        :style="{ width: `${percent}%` }"
        data-testid="notification-bar"
      />
    </div>
  </div>
</template>
