<script setup lang="ts">
import { computed } from 'vue'
import { RadioIcon, RefreshCwIcon, WifiOffIcon } from '@lucide/vue'
import type { DeliveryConnection } from '@/delivery/useDeliveryStream'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

const props = defineProps<{
  connection: DeliveryConnection
  /** Ardışık başarısız bağlantı denemesi (yeniden bağlanırken gösterilir). */
  failures: number
  /** Son durum görüntüsünün alındığı an; hiç gelmediyse null. */
  updatedAt: Date | null
  /** Bağlantı kalıcı olarak koptuğunda hatanın başlığı. */
  errorTitle: string | null
}>()
defineEmits<{ retry: [] }>()

const timeFormat = new Intl.DateTimeFormat('tr-TR', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})
const updatedText = computed(() => (props.updatedAt ? timeFormat.format(props.updatedAt) : ''))

/**
 * Ekran okuyuculara bildirilen metin. Saniyelik güncellemeleri (son güncelleme
 * saati) içermez: yalnızca bağlantı durumu değişince okunur.
 */
const announcement = computed(() => {
  switch (props.connection) {
    case 'connecting':
      return 'Canlı takibe bağlanılıyor.'
    case 'live':
      return 'Canlı takip bağlı.'
    case 'reconnecting':
      return `Bağlantı koptu, yeniden bağlanılıyor. Deneme ${props.failures}.`
    case 'completed':
      return 'Gönderim tamamlandı. Canlı takip kapatıldı.'
    default:
      return ''
  }
})
</script>

<template>
  <Alert v-if="connection === 'failed'" variant="destructive" data-testid="connection-failed">
    <WifiOffIcon aria-hidden="true" />
    <AlertTitle>{{ errorTitle ?? 'Canlı takip başlatılamadı.' }}</AlertTitle>
    <AlertDescription>
      <p v-if="updatedAt">Gördüğünüz sayılar en son alınan durumdur ({{ updatedText }}).</p>
      <Button type="button" variant="outline" size="sm" @click="$emit('retry')">
        <RefreshCwIcon aria-hidden="true" />
        Yeniden bağlan
      </Button>
    </AlertDescription>
  </Alert>

  <div v-else-if="connection !== 'idle'" data-testid="connection-status">
    <!-- Ekran okuyucu için tek kaynak: yalnızca bağlantı durumu değişince okunur.
         Aşağıdaki görünür metin aynı bilgiyi (ve saniyelik saati) gösterdiği için
         ekran okuyucudan gizlidir; aksi halde her şey iki kez okunurdu. -->
    <span class="sr-only" role="status">{{ announcement }}</span>

    <div class="text-muted-foreground flex flex-wrap items-center gap-2 text-sm" aria-hidden="true">
      <template v-if="connection === 'connecting'">
        <Spinner class="motion-reduce:animate-none" />
        <span>Canlı takibe bağlanılıyor…</span>
      </template>

      <template v-else-if="connection === 'live'">
        <Badge
          variant="outline"
          class="border-transparent bg-emerald-100 text-emerald-900 dark:bg-emerald-400/15 dark:text-emerald-300"
        >
          <RadioIcon aria-hidden="true" />
          Canlı
        </Badge>
        <span>Son güncelleme: {{ updatedText }}</span>
      </template>

      <template v-else-if="connection === 'reconnecting'">
        <Spinner class="motion-reduce:animate-none" />
        <span>
          Bağlantı koptu, yeniden bağlanılıyor (deneme {{ failures }}). Gördüğünüz sayılar son
          bilinen durumdur<template v-if="updatedAt"> ({{ updatedText }})</template>.
        </span>
      </template>

      <template v-else-if="connection === 'completed'">
        <!-- "Tamamlandı" kampanya durumunun rozetidir; bağlantı rozeti onunla karışmasın. -->
        <Badge variant="outline">Kapandı</Badge>
        <span>Canlı takip kapatıldı. Son güncelleme: {{ updatedText }}</span>
      </template>
    </div>
  </div>
</template>
