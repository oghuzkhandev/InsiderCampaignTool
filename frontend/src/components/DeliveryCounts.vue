<script setup lang="ts">
import { computed } from 'vue'
import type { DeliverySnapshot } from '@/api/delivery'
import { COUNT_ITEMS, exhaustedNote, rejectedNote } from '@/delivery/messages'
import { finishedMessages, finishedPercent, totalMessages } from '@/delivery/summary'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'

const props = defineProps<{ snapshot: DeliverySnapshot }>()

const nf = new Intl.NumberFormat('tr-TR')
const total = computed(() => totalMessages(props.snapshot.messages))
const finished = computed(() => finishedMessages(props.snapshot.messages))
const percent = computed(() => finishedPercent(props.snapshot.messages))
</script>

<template>
  <Card role="region" aria-labelledby="delivery-counts-title">
    <CardHeader>
      <CardTitle>
        <h2 id="delivery-counts-title">Mesaj durumu</h2>
      </CardTitle>
      <CardDescription>
        Toplam <span data-testid="total-messages">{{ nf.format(total) }}</span> mesaj
      </CardDescription>
    </CardHeader>

    <CardContent class="grid gap-4">
      <div class="grid gap-2">
        <Progress :model-value="percent" aria-label="İşi biten mesajların oranı" />
        <p class="text-muted-foreground text-sm" data-testid="progress-text">
          %{{ percent }} tamamlandı · {{ nf.format(finished) }} / {{ nf.format(total) }} mesajın işi
          bitti
        </p>
      </div>

      <dl class="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <div v-for="item in COUNT_ITEMS" :key="item.key" class="rounded-lg border p-3">
          <dt class="text-muted-foreground text-sm">{{ item.label }}</dt>
          <dd
            class="text-2xl font-bold tabular-nums"
            :class="{ 'text-destructive': item.key === 'failed' && snapshot.messages.failed > 0 }"
            :data-testid="`count-${item.key}`"
          >
            {{ nf.format(snapshot.messages[item.key]) }}
          </dd>
          <dd class="text-muted-foreground mt-1 text-xs">{{ item.hint }}</dd>
        </div>
      </dl>

      <!-- role="note": sayılar sürekli güncellenir; canlı bölge (alert) olsaydı her
           güncellemede ekran okuyucuya okunurdu. -->
      <Alert v-if="snapshot.failures.exhausted > 0" role="note" data-testid="note-exhausted">
        <AlertDescription>{{ exhaustedNote(snapshot.failures.exhausted) }}</AlertDescription>
      </Alert>
      <Alert
        v-if="snapshot.failures.rejected > 0"
        variant="destructive"
        role="note"
        data-testid="note-rejected"
      >
        <AlertDescription>{{ rejectedNote(snapshot.failures.rejected) }}</AlertDescription>
      </Alert>
    </CardContent>
  </Card>
</template>
