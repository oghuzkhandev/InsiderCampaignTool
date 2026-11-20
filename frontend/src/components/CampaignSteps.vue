<script setup lang="ts">
import { CheckIcon } from '@lucide/vue'
import { STEPS, stepStatusText, type StepKey, type StepStatus } from '@/campaign/steps'

// Kitle → İçerik → Onay → Gönderim. Yalnızca bilgi verir, tıklanamaz; bu yüzden etkileşimli
// bir sihirbaz bileşeni yerine düz bir sıralı liste kullanılır. Durum yalnızca renkle değil,
// işaretle (tamamlanan adımda tik) ve ekran okuyucu metniyle de anlatılır.
defineProps<{ statuses: Record<StepKey, StepStatus> }>()
</script>

<template>
  <ol class="grid grid-cols-4 gap-2" aria-label="Kampanya adımları" data-testid="campaign-steps">
    <li
      v-for="(step, index) in STEPS"
      :key="step.key"
      class="grid gap-1.5"
      :data-step="step.key"
      :data-status="statuses[step.key]"
      :aria-current="statuses[step.key] === 'current' ? 'step' : undefined"
    >
      <span
        class="h-1 rounded-full"
        :class="statuses[step.key] === 'upcoming' ? 'bg-border' : 'bg-primary'"
        aria-hidden="true"
      />
      <span
        class="flex items-center gap-1 text-xs sm:gap-1.5 sm:text-sm"
        :class="
          statuses[step.key] === 'upcoming'
            ? 'text-muted-foreground'
            : statuses[step.key] === 'current'
              ? 'font-semibold'
              : 'font-medium'
        "
      >
        <CheckIcon
          v-if="statuses[step.key] === 'done'"
          class="size-3.5 shrink-0"
          aria-hidden="true"
        />
        <span v-else class="tabular-nums" aria-hidden="true">{{ index + 1 }}.</span>
        <span>{{ step.label }}</span>
        <span class="sr-only">({{ stepStatusText(statuses[step.key]) }})</span>
      </span>
    </li>
  </ol>
</template>
