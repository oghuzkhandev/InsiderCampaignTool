<script setup lang="ts">
import { formatScheduledTime, deviceTimeZone } from '@/campaign/schedule'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
defineProps<{ at: string; status: string }>()
</script>
<template>
  <Alert role="note" data-testid="schedule-notice">
    <AlertTitle>{{
      status === 'approved' ? 'Gönderim planlandı' : 'Planlanan gönderim zamanı'
    }}</AlertTitle>
    <AlertDescription>
      <p>
        <time :datetime="at">{{ formatScheduledTime(at) }}</time> · {{ deviceTimeZone() }}
      </p>
      <p v-if="status === 'approved'">
        Mesajlar bu saatten önce gönderilmez. Worker hazır olduğunda gönderim başlayacak; hız
        sınırları nedeniyle tamamlanması zamana yayılabilir.
      </p>
    </AlertDescription>
  </Alert>
</template>
