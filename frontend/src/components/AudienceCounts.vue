<script setup lang="ts">
import { computed } from 'vue'
import { MailIcon, MessageSquareIcon, UsersIcon } from '@lucide/vue'
import type { Audience } from '@/api/campaigns'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const props = defineProps<{
  audience: Audience
  /** Yan sütun için kompakt görünüm: sayılar satır satır, daha az boşluk. Veriler aynıdır. */
  compact?: boolean
}>()

const nf = new Intl.NumberFormat('tr-TR')
const hasRecipients = computed(() => props.audience.sms > 0 || props.audience.email > 0)
const items = computed(() => [
  { id: 'total', label: 'Toplam eşleşen kişi', value: props.audience.total, icon: UsersIcon },
  { id: 'sms', label: "SMS'e uygun kişi", value: props.audience.sms, icon: MessageSquareIcon },
  { id: 'email', label: 'E-postaya uygun kişi', value: props.audience.email, icon: MailIcon },
])
</script>

<template>
  <Card role="region" aria-labelledby="audience-title" :size="compact ? 'sm' : 'default'">
    <CardHeader>
      <CardTitle class="flex items-center gap-2">
        <UsersIcon class="text-primary size-5" aria-hidden="true" />
        <h2 id="audience-title">Kitle önizleme</h2>
      </CardTitle>
    </CardHeader>

    <CardContent class="grid gap-4">
      <dl v-if="compact" class="grid gap-2 text-sm" data-testid="audience-compact">
        <div
          v-for="item in items"
          :key="item.id"
          class="flex items-baseline justify-between gap-3 border-b pb-2 last:border-b-0 last:pb-0"
        >
          <dt class="text-muted-foreground flex items-center gap-2">
            <component :is="item.icon" class="size-4 shrink-0" aria-hidden="true" />
            {{ item.label }}
          </dt>
          <dd class="text-lg font-bold tabular-nums" :data-testid="item.id">
            {{ nf.format(item.value) }}
          </dd>
        </div>
      </dl>
      <dl v-else class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div v-for="item in items" :key="item.id" class="bg-muted/50 rounded-lg border p-3">
          <dt class="text-muted-foreground flex items-center gap-2 text-sm">
            <component :is="item.icon" class="text-primary size-4 shrink-0" aria-hidden="true" />
            {{ item.label }}
          </dt>
          <dd class="text-2xl font-bold tabular-nums" :data-testid="item.id">
            {{ nf.format(item.value) }}
          </dd>
        </div>
      </dl>

      <p
        class="text-muted-foreground"
        :class="compact ? 'text-xs' : 'text-sm'"
        data-testid="audience-note"
      >
        Kanal sayıları yalnızca ilgili kanal için açıkça izin vermiş ve o kanalın adresi (telefon
        veya e-posta) bulunan kişileri içerir. İki kanala da uygun kişi ikisinde de sayılır; bu
        yüzden kanal sayılarının toplamı toplam kişi sayısı değildir.
      </p>

      <Alert v-if="audience.total === 0" role="status" data-testid="audience-warning">
        <AlertDescription>Bu tanımla hiç kullanıcı eşleşmedi.</AlertDescription>
      </Alert>
      <Alert v-else-if="!hasRecipients" role="status" data-testid="audience-warning">
        <AlertDescription>
          Eşleşen kişiler var ancak hiçbir kanalda izinli alıcı yok. Bu kampanya gönderime
          onaylanamaz.
        </AlertDescription>
      </Alert>
    </CardContent>
  </Card>
</template>
