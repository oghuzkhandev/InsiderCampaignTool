<script setup lang="ts">
import { computed } from 'vue'
import {
  ActivityIcon,
  CircleCheckBigIcon,
  CircleXIcon,
  HourglassIcon,
  MailIcon,
  MessageSquareIcon,
} from '@lucide/vue'
import { CHANNEL_KEYS, type ChannelKey, type DeliverySnapshot } from '@/api/delivery'
import { COUNT_ITEMS, exhaustedNote, rejectedNote } from '@/delivery/messages'
import {
  finishedMessages,
  finishedPercent,
  totalMessages,
  unfinishedMessages,
} from '@/delivery/summary'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'

const props = defineProps<{ snapshot: DeliverySnapshot }>()

const nf = new Intl.NumberFormat('tr-TR')
const total = computed(() => totalMessages(props.snapshot.messages))
const finished = computed(() => finishedMessages(props.snapshot.messages))
const percent = computed(() => finishedPercent(props.snapshot.messages))
const unfinished = computed(() => unfinishedMessages(props.snapshot.messages))

const recovered = computed(() => props.snapshot.recovered ?? 0)

const CHANNEL_LABELS: Record<ChannelKey, string> = { sms: 'SMS', email: 'E-posta' }
const CHANNEL_ICONS = { sms: MessageSquareIcon, email: MailIcon } as const

/** Kanal başına: toplam, işi biten oranı ve üç ana sayı (gönderildi, başarısız, tamamlanmamış). */
const channelRows = computed(() => {
  const channels = props.snapshot.channels
  if (!channels) return []
  return CHANNEL_KEYS.map((key) => {
    const counts = channels[key]
    return {
      key,
      label: CHANNEL_LABELS[key],
      icon: CHANNEL_ICONS[key],
      total: totalMessages(counts),
      percent: finishedPercent(counts),
      skipped: counts.skipped,
      metrics: [
        { key: 'sent', label: 'Gönderildi', value: counts.sent },
        { key: 'failed', label: 'Başarısız', value: counts.failed },
        { key: 'unfinished', label: 'Tamamlanmamış', value: unfinishedMessages(counts) },
      ],
    }
  })
})

/** Üstteki üç özet sayı: kullanıcının ilk sorusu "ne kadarı gitti, ne kadarı gitmedi, ne kadarının işi sürüyor". */
const summary = computed(() => [
  {
    id: 'sent',
    label: 'Gönderildi',
    value: props.snapshot.messages.sent,
    hint: 'Sağlayıcı kabul etti',
    danger: false,
    icon: CircleCheckBigIcon,
    tone: 'bg-emerald-50 border-emerald-200 dark:bg-emerald-400/10 dark:border-emerald-400/25',
    iconTone: 'text-emerald-700 dark:text-emerald-300',
  },
  {
    id: 'failed',
    label: 'Başarısız',
    value: props.snapshot.messages.failed,
    hint: 'Gönderilemedi, yeniden denenmeyecek',
    danger: props.snapshot.messages.failed > 0,
    icon: CircleXIcon,
    tone: 'bg-rose-50 border-rose-200 dark:bg-rose-400/10 dark:border-rose-400/25',
    iconTone: 'text-rose-700 dark:text-rose-300',
  },
  {
    id: 'unfinished',
    label: 'Tamamlanmamış',
    value: unfinished.value,
    hint: 'Sırada, yeniden denenecek veya gönderiliyor',
    danger: false,
    icon: HourglassIcon,
    tone: 'bg-amber-50 border-amber-200 dark:bg-amber-400/10 dark:border-amber-400/25',
    iconTone: 'text-amber-700 dark:text-amber-300',
  },
])
</script>

<template>
  <Card role="region" aria-labelledby="delivery-counts-title">
    <CardHeader>
      <CardTitle class="flex items-center gap-2">
        <ActivityIcon class="text-primary size-5" aria-hidden="true" />
        <h2 id="delivery-counts-title">Mesaj durumu</h2>
      </CardTitle>
      <CardDescription>
        Toplam <span data-testid="total-messages">{{ nf.format(total) }}</span> mesaj
      </CardDescription>
    </CardHeader>

    <CardContent class="grid gap-6">
      <dl class="grid grid-cols-3 gap-3" data-testid="delivery-summary">
        <div
          v-for="item in summary"
          :key="item.id"
          class="rounded-lg border p-3 sm:p-4"
          :class="item.tone"
        >
          <dt class="text-muted-foreground flex items-center gap-1.5 text-sm">
            <component
              :is="item.icon"
              class="size-4 shrink-0"
              :class="item.iconTone"
              aria-hidden="true"
            />
            {{ item.label }}
          </dt>
          <dd
            class="text-2xl font-bold tabular-nums sm:text-3xl"
            :class="{ 'text-destructive': item.danger }"
            :data-testid="`summary-${item.id}`"
          >
            {{ nf.format(item.value) }}
          </dd>
          <dd class="text-muted-foreground mt-1 hidden text-xs sm:block">{{ item.hint }}</dd>
        </div>
      </dl>

      <div class="grid gap-2">
        <Progress :model-value="percent" aria-label="İşi biten mesajların oranı" />
        <p class="text-muted-foreground text-sm" data-testid="progress-text">
          %{{ percent }} tamamlandı · {{ nf.format(finished) }} / {{ nf.format(total) }} mesajın işi
          bitti
        </p>
        <!-- Yeniden denemeyle toparlanan: birden fazla denemede sonuçlanan gönderilenler. İlk
             denemenin neden tamamlanamadığı ayrılmaz (hata, kayıp yanıt veya çöken worker). -->
        <p v-if="recovered > 0" class="text-sm" data-testid="recovered-note">
          <span class="font-medium">Yeniden denemeyle toparlanan:</span>
          <span class="font-bold tabular-nums" data-testid="recovered-count">
            {{ nf.format(recovered) }}
          </span>
          mesaj
          <span class="text-muted-foreground">
            (ilk denemede tamamlanamadı, yeniden denemeyle gönderildi)
          </span>
        </p>
      </div>

      <!-- Kanal dağılımı: SMS ve e-posta ayrı ayrı. Sunucu her görüntüde yollar; eski bir sunucu
           yollamazsa bölüm hiç görünmez. -->
      <div v-if="channelRows.length > 0" class="grid gap-3" data-testid="channel-split">
        <h3 class="text-sm font-medium">Kanal dağılımı</h3>
        <div class="grid gap-3 sm:grid-cols-2">
          <section
            v-for="row in channelRows"
            :key="row.key"
            class="grid content-start gap-2 rounded-lg border p-3"
            :aria-label="`${row.label} gönderim durumu`"
            :data-testid="`channel-${row.key}`"
          >
            <div class="flex items-baseline justify-between gap-2">
              <p class="flex items-center gap-1.5 text-sm font-medium">
                <component :is="row.icon" class="text-primary size-4" aria-hidden="true" />
                {{ row.label }}
              </p>
              <p class="text-muted-foreground text-xs" :data-testid="`channel-${row.key}-total`">
                {{ nf.format(row.total) }} mesaj
              </p>
            </div>

            <p
              v-if="row.total === 0"
              class="text-muted-foreground text-sm"
              :data-testid="`channel-${row.key}-empty`"
            >
              Bu kanalda mesaj yok.
            </p>
            <template v-else>
              <Progress
                :model-value="row.percent"
                :aria-label="`${row.label}: işi biten mesajların oranı`"
              />
              <dl class="grid grid-cols-3 gap-2">
                <div v-for="metric in row.metrics" :key="metric.key">
                  <dt class="text-muted-foreground text-xs">{{ metric.label }}</dt>
                  <dd
                    class="text-lg font-bold tabular-nums"
                    :class="{ 'text-destructive': metric.key === 'failed' && metric.value > 0 }"
                    :data-testid="`channel-${row.key}-${metric.key}`"
                  >
                    {{ nf.format(metric.value) }}
                  </dd>
                </div>
              </dl>
              <p
                v-if="row.skipped > 0"
                class="text-muted-foreground text-xs"
                :data-testid="`channel-${row.key}-skipped`"
              >
                Atlandı: {{ nf.format(row.skipped) }}
              </p>
            </template>
          </section>
        </div>
      </div>

      <div class="grid gap-3">
        <h3 class="text-sm font-medium">Ayrıntı</h3>
        <dl class="grid grid-cols-2 gap-3 sm:grid-cols-3" data-testid="count-details">
          <div v-for="item in COUNT_ITEMS" :key="item.key" class="rounded-lg border p-3">
            <dt class="text-muted-foreground text-sm">{{ item.label }}</dt>
            <dd
              class="text-xl font-bold tabular-nums"
              :class="{ 'text-destructive': item.key === 'failed' && snapshot.messages.failed > 0 }"
              :data-testid="`count-${item.key}`"
            >
              {{ nf.format(snapshot.messages[item.key]) }}
            </dd>
            <dd class="text-muted-foreground mt-1 text-xs">{{ item.hint }}</dd>
          </div>
        </dl>
      </div>

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
