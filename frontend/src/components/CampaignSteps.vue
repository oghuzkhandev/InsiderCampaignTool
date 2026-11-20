<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { CheckIcon, PenLineIcon, SendIcon, ShieldCheckIcon, UsersRoundIcon } from '@lucide/vue'
import { STEPS, stepStatusText, type StepKey, type StepStatus } from '@/campaign/steps'

// Kitle → İçerik → Onay → Gönderim. Yalnızca bilgi verir, tıklanamaz; bu yüzden etkileşimli
// bir sihirbaz bileşeni yerine düz bir sıralı liste kullanılır. Her adım numaralı bir dairedir:
// biten adım yeşil ve tikli, şu anki adım ana renkte ve yumuşakça nabız atar, bekleyen adım
// soluk ve çerçeveli. Adımlar arasındaki çizgi, önceki adım bitince soldan sağa dolar; durum
// değişince renkler ve çizgi yumuşakça geçer. Durum yalnızca renkle değil, numara/tik, kısa
// yazı ve ekran okuyucu metniyle de anlatılır. Hareket, "hareketi azalt" tercihinde kapanır.
// Dört eşit sütunda daireler ve etiketler ortalanır; ilk ve son adımın kenar boşluğu aynıdır.
// Bağlantı çizgisi, her dairenin sağından sonraki dairenin soluna uzanır.
const props = defineProps<{ statuses: Record<StepKey, StepStatus> }>()

// Sayfa açılırken çizgiler boş başlayıp dolsun diye ilk çizimden hemen sonra açılır.
const ready = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null
onMounted(() => {
  timer = setTimeout(() => (ready.value = true), 60)
})
onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
})

const CAPTIONS: Record<StepStatus, string> = {
  done: 'Tamamlandı',
  current: 'Devam ediyor',
  upcoming: 'Bekliyor',
}

// Adımın anlamına uygun ikon (yalnızca süs; ad zaten yazıyor).
const STEP_ICONS: Record<StepKey, typeof UsersRoundIcon> = {
  audience: UsersRoundIcon,
  content: PenLineIcon,
  approval: ShieldCheckIcon,
  delivery: SendIcon,
}

const circleTone: Record<StepStatus, string> = {
  done: 'border-transparent bg-emerald-600 text-white dark:bg-emerald-500 dark:text-emerald-950',
  current: 'border-transparent bg-primary text-primary-foreground',
  upcoming: 'border-border bg-background text-muted-foreground',
}
const labelTone: Record<StepStatus, string> = {
  done: 'font-medium text-foreground',
  current: 'font-semibold text-foreground',
  upcoming: 'font-medium text-muted-foreground',
}
const captionTone: Record<StepStatus, string> = {
  done: 'text-emerald-700 dark:text-emerald-300',
  current: 'text-primary',
  upcoming: 'text-muted-foreground',
}
</script>

<template>
  <ol
    class="grid w-full grid-cols-4 px-1 py-1"
    aria-label="Kampanya adımları"
    data-testid="campaign-steps"
  >
    <li
      v-for="(step, index) in STEPS"
      :key="step.key"
      class="campaign-step relative grid min-w-0 content-start px-1 text-center"
      :data-step="step.key"
      :data-status="props.statuses[step.key]"
      :aria-current="props.statuses[step.key] === 'current' ? 'step' : undefined"
    >
      <!-- Daire ve sonraki adıma uzanan çizgi. Son adımın çizgisi yoktur. -->
      <span class="flex items-center justify-center" aria-hidden="true">
        <span
          class="relative grid size-8 shrink-0 place-items-center rounded-full border-2 text-sm font-semibold transition-[background-color,border-color,color,box-shadow,transform] duration-500 ease-out motion-reduce:transition-none sm:size-9"
          :class="circleTone[props.statuses[step.key]]"
          data-testid="step-circle"
        >
          <!-- Halka: daireden 4 px taşar ve yavaşça nabız atar (boyut değişmez, yalnızca soluklaşır). -->
          <span
            v-if="props.statuses[step.key] === 'current'"
            class="bg-primary/25 absolute -inset-1 rounded-full animate-pulse animation-duration-[2.4s] motion-reduce:animate-none"
            data-testid="step-pulse"
          />
          <CheckIcon
            v-if="props.statuses[step.key] === 'done'"
            class="animate-in zoom-in-50 spin-in-12 relative size-4 duration-500 motion-reduce:animate-none sm:size-[1.125rem]"
            :stroke-width="3"
          />
          <span v-else class="relative tabular-nums">{{ index + 1 }}</span>
        </span>

        <span
          v-if="index < STEPS.length - 1"
          class="bg-border absolute top-4 left-[calc(50%+1.375rem)] h-0.5 w-[calc(100%-2.75rem)] -translate-y-1/2 overflow-hidden rounded-full sm:top-[1.125rem] sm:left-[calc(50%+1.625rem)] sm:w-[calc(100%-3.25rem)]"
          :data-filled="props.statuses[step.key] === 'done'"
          data-testid="step-connector"
        >
          <span
            class="absolute inset-0 origin-left rounded-full bg-emerald-600 transition-transform duration-700 ease-out motion-reduce:transition-none dark:bg-emerald-500"
            :class="ready && props.statuses[step.key] === 'done' ? 'scale-x-100' : 'scale-x-0'"
          />
        </span>
      </span>

      <span class="mt-2 grid min-w-0 gap-0.5 justify-items-center">
        <span
          class="flex items-center justify-center gap-1.5 text-xs transition-colors duration-500 sm:text-sm"
          :class="labelTone[props.statuses[step.key]]"
        >
          <component
            :is="STEP_ICONS[step.key]"
            class="hidden size-4 shrink-0 sm:block"
            aria-hidden="true"
            data-testid="step-icon"
          />
          {{ step.label }}
        </span>
        <span
          class="hidden text-xs transition-colors duration-500 sm:block"
          :class="captionTone[props.statuses[step.key]]"
          aria-hidden="true"
          data-testid="step-caption"
        >
          {{ CAPTIONS[props.statuses[step.key]] }}
        </span>
        <span class="sr-only">({{ stepStatusText(props.statuses[step.key]) }})</span>
      </span>
    </li>
  </ol>
</template>
