<script setup lang="ts">
import { CircleAlertIcon, RefreshCwIcon } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

withDefaults(
  defineProps<{
    title: string
    reasons: string[]
    retryable: boolean
    /** Sebepler varken gösterilen yönlendirme; boş string gizler. */
    hint?: string
    retryLabel?: string
  }>(),
  { hint: "Brief'i düzenleyip yeniden deneyebilirsiniz.", retryLabel: 'Yeniden dene' },
)
defineEmits<{ retry: [] }>()
</script>

<template>
  <!-- Alert kendiliğinden role="alert" taşır: hata ekran okuyucuya hemen bildirilir. -->
  <!-- `wrap-anywhere`: model veya sunucudan gelen boşluksuz uzun metin kartı taşırmaz. -->
  <Alert variant="destructive" class="wrap-anywhere" data-testid="error-panel">
    <CircleAlertIcon aria-hidden="true" />
    <AlertTitle>{{ title }}</AlertTitle>
    <AlertDescription>
      <ul v-if="reasons.length > 0" class="list-disc space-y-1 pl-5">
        <li v-for="(reason, i) in reasons" :key="i">{{ reason }}</li>
      </ul>
      <p v-if="reasons.length > 0 && hint" class="mt-2">{{ hint }}</p>
      <Button
        v-if="retryable"
        type="button"
        variant="outline"
        size="sm"
        class="mt-3"
        data-testid="error-retry"
        @click="$emit('retry')"
      >
        <RefreshCwIcon aria-hidden="true" />
        {{ retryLabel }}
      </Button>
    </AlertDescription>
  </Alert>
</template>
