<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronRightIcon, MailIcon, MessageSquareIcon, UsersRoundIcon } from '@lucide/vue'
import type { DeliverySnapshot } from '@/api/delivery'
import { totalMessages } from '@/delivery/summary'
import RecipientsSheet from '@/components/RecipientsSheet.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

// "Alıcılar" kartı: mesaj sayısı ve kanal dağılımı ile birlikte, kimin mesaj aldığını gösteren
// çekmeceyi (sağdan açılan panel) açan düğme. Sayılar canlı görüntüden gelir; görüntü henüz yoksa
// düğme yine de çalışır. Gönderim sürerken çekmecedeki liste kendiliğinden tazelenir.
const props = defineProps<{
  campaignId: string
  snapshot: DeliverySnapshot | null
  /** Gönderim sürüyor mu (liste tazelensin mi)? */
  live: boolean
}>()

const open = ref(false)
const nf = new Intl.NumberFormat('tr-TR')
const total = computed(() => (props.snapshot ? totalMessages(props.snapshot.messages) : null))
const channels = computed(() => {
  const split = props.snapshot?.channels
  if (!split) return []
  return [
    { key: 'sms', label: 'SMS', icon: MessageSquareIcon, count: totalMessages(split.sms) },
    { key: 'email', label: 'E-posta', icon: MailIcon, count: totalMessages(split.email) },
  ]
})
</script>

<template>
  <Card
    role="region"
    aria-labelledby="recipients-card-title"
    class="from-primary/5 via-card to-card bg-linear-to-br"
    data-testid="recipients-card"
  >
    <CardHeader>
      <CardTitle class="flex items-center gap-2">
        <UsersRoundIcon class="text-primary size-5" aria-hidden="true" />
        <h2 id="recipients-card-title">Alıcılar</h2>
      </CardTitle>
      <CardDescription>Bu kampanyada kimin mesaj aldığını görün.</CardDescription>
    </CardHeader>
    <CardContent class="grid gap-4">
      <div v-if="total !== null" class="flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
        <p class="text-3xl leading-none font-bold tabular-nums" data-testid="recipients-count">
          {{ nf.format(total) }}
          <span class="text-muted-foreground text-sm font-normal">mesaj</span>
        </p>
        <ul class="text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs">
          <li v-for="c in channels" :key="c.key" class="flex items-center gap-1.5">
            <component :is="c.icon" class="size-3.5" aria-hidden="true" />
            {{ c.label }}
            <span class="text-foreground font-medium tabular-nums">{{ nf.format(c.count) }}</span>
          </li>
        </ul>
      </div>

      <Button
        type="button"
        size="lg"
        class="w-full justify-between"
        aria-haspopup="dialog"
        data-testid="recipients-open"
        @click="open = true"
      >
        <span class="flex items-center gap-2">
          <UsersRoundIcon aria-hidden="true" />
          Alıcıları görüntüle
        </span>
        <ChevronRightIcon aria-hidden="true" />
      </Button>
    </CardContent>

    <RecipientsSheet v-model:open="open" :campaign-id="campaignId" :live="live" />
  </Card>
</template>
