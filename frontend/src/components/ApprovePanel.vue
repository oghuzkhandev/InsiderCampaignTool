<script setup lang="ts">
import { ref, watch } from 'vue'
import { localScheduleToUTC, deviceTimeZone, formatScheduledTime } from '@/campaign/schedule'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  CheckIcon,
  CalendarClockIcon,
  InfoIcon,
  MailIcon,
  MessageSquareIcon,
  ShieldCheckIcon,
  UsersRoundIcon,
  XIcon,
} from '@lucide/vue'
import type { Audience } from '@/api/campaigns'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

const props = defineProps<{
  /** Onaylanacak içerik sürümü. */
  version: number
  /** Önizlemedeki kitle sayıları; kesin sayı onay anında belirlenir. */
  audience: Audience
  /** Onay şu an yapılamaz (kaydedilmemiş değişiklik, işlem sürüyor). */
  disabled: boolean
  pending: boolean
}>()
const emit = defineEmits<{ approve: [scheduledAt?: string] }>()

const nf = new Intl.NumberFormat('tr-TR')
const confirming = ref(false)
const sendLater = ref(false)
const localTime = ref('')
const scheduleError = ref('')
const confirmedTime = ref<string | undefined>(undefined)

watch([sendLater, localTime], () => {
  confirming.value = false
  scheduleError.value = ''
})

function openConfirmation() {
  if (props.disabled || props.pending) return
  scheduleError.value = ''
  confirmedTime.value = undefined
  if (sendLater.value) {
    const result = localScheduleToUTC(localTime.value)
    if (!result.ok) {
      scheduleError.value = result.message
      return
    }
    confirmedTime.value = result.utc
  }
  confirming.value = true
}

// Sürüm değişirse (yeni kayıt) önceki sürüm için açılmış onay isteği geçersizdir.
watch(
  () => props.version,
  () => {
    confirming.value = false
  },
)
watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) confirming.value = false
  },
)

function confirm() {
  if (props.disabled || props.pending) return
  if (sendLater.value) {
    const result = localScheduleToUTC(localTime.value)
    if (!result.ok) {
      confirming.value = false
      scheduleError.value = result.message
      return
    }
  }
  confirming.value = false
  if (confirmedTime.value) emit('approve', confirmedTime.value)
  else emit('approve')
}
</script>

<template>
  <div class="grid min-w-0 gap-6">
    <Card role="region" aria-labelledby="approve-title">
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <ShieldCheckIcon class="text-primary size-5" aria-hidden="true" />
          <h2 id="approve-title">Onay</h2>
        </CardTitle>
        <CardDescription data-testid="approve-version">
          Sürüm {{ version }} onaylanacak.
        </CardDescription>
      </CardHeader>

      <CardContent class="grid gap-6">
        <!-- Onaydan önce görülmesi gerekenler, ayraçlarla bölümlenmiş tek akışta: kimlere, ne gidecek,
           ve onay eylemi. -->
        <Separator />

        <section class="grid gap-3" aria-labelledby="approve-recipients-title">
          <h3 id="approve-recipients-title" class="flex items-center gap-2 text-sm font-medium">
            <UsersRoundIcon class="text-muted-foreground size-4" aria-hidden="true" />
            Alıcılar
          </h3>
          <dl class="divide-y rounded-lg border text-sm" data-testid="approve-recipients">
            <div class="flex items-center justify-between gap-3 px-3 py-2.5">
              <dt class="text-muted-foreground flex items-center gap-2">
                <MessageSquareIcon class="size-4 shrink-0" aria-hidden="true" />
                Tahmini SMS alıcısı
              </dt>
              <dd class="font-bold tabular-nums" data-testid="approve-sms">
                ≈ {{ nf.format(audience.sms) }}
              </dd>
            </div>
            <div class="flex items-center justify-between gap-3 px-3 py-2.5">
              <dt class="text-muted-foreground flex items-center gap-2">
                <MailIcon class="size-4 shrink-0" aria-hidden="true" />
                Tahmini e-posta alıcısı
              </dt>
              <dd class="font-bold tabular-nums" data-testid="approve-email">
                ≈ {{ nf.format(audience.email) }}
              </dd>
            </div>
          </dl>
          <p class="text-muted-foreground text-xs">
            Sayılar önizlemeye göredir; kesin sayı onay anındaki izin ve adres durumuna göre
            belirlenir.
          </p>
        </section>

        <Separator />

        <section
          class="grid min-w-0 gap-4"
          aria-labelledby="approve-schedule-title"
          data-testid="approve-schedule"
        >
          <div class="flex items-center justify-between gap-3">
            <h3 id="approve-schedule-title" class="flex items-center gap-2 text-sm font-semibold">
              <CalendarClockIcon class="text-primary size-4 shrink-0" aria-hidden="true" />
              Gönderim zamanı
            </h3>
          </div>
          <p class="text-muted-foreground text-sm leading-relaxed">
            Mesajların ne zaman gönderilmeye başlayacağını seçin. Gönderim yalnızca son onayınızdan
            sonra başlatılır veya planlanır.
          </p>
          <RadioGroup
            :model-value="sendLater ? 'later' : 'now'"
            :disabled="disabled || pending"
            aria-labelledby="approve-schedule-title"
            @update:model-value="sendLater = $event === 'later'"
          >
            <div class="bg-background/50 flex items-start gap-3 rounded-lg border p-3">
              <RadioGroupItem
                id="send-now"
                value="now"
                class="mt-0.5"
                aria-describedby="send-now-hint"
              />
              <div class="grid min-w-0 gap-1.5">
                <Label for="send-now">Hemen gönder</Label>
                <p id="send-now-hint" class="text-muted-foreground text-xs leading-relaxed">
                  Son onayınızdan sonra gönderim başlatılır.
                </p>
              </div>
              <TooltipProvider :delay-duration="150">
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      class="ml-auto shrink-0"
                      :disabled="pending"
                      aria-label="Hemen gönder hakkında bilgi"
                    >
                      <InfoIcon class="size-4" aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-xs text-sm leading-relaxed">
                    Son onayınızdan sonra mesajlar sırayla gönderilir. Tüm mesajların gönderilmesi
                    alıcı sayısına göre biraz zaman alabilir. Gönderim durumunu canlı takip
                    ekranından izleyebilirsiniz.
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
            <div class="bg-background/50 flex items-start gap-3 rounded-lg border p-3">
              <RadioGroupItem
                id="send-later"
                value="later"
                class="mt-0.5"
                aria-describedby="send-later-hint"
              />
              <div class="grid min-w-0 gap-1.5">
                <Label for="send-later">Tarih ve saat seç</Label>
                <p id="send-later-hint" class="text-muted-foreground text-xs leading-relaxed">
                  Mesajlar seçtiğiniz saate kadar kuyrukta bekler, daha erken gönderilmez.
                </p>
              </div>
              <TooltipProvider :delay-duration="150">
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      class="ml-auto shrink-0"
                      :disabled="pending"
                      aria-label="Tarih ve saat seç hakkında bilgi"
                    >
                      <InfoIcon class="size-4" aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-xs text-sm leading-relaxed">
                    Seçtiğiniz tarih ve saat, cihazınızın saat dilimine göredir. Mesajlar bu saatten
                    önce gönderilmez. Alıcılar son onayınızda belirlenir; onaydan sonra gönderim
                    zamanı değiştirilemez.
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          </RadioGroup>
          <div v-if="sendLater" class="grid min-w-0 gap-2">
            <Label for="send-time">Gönderim tarihi ve saati</Label>
            <Input
              id="send-time"
              v-model="localTime"
              type="datetime-local"
              step="60"
              :disabled="disabled || pending"
              :aria-invalid="Boolean(scheduleError)"
              aria-describedby="send-time-zone send-time-error"
              class="min-w-0"
            />
            <p id="send-time-zone" class="text-muted-foreground text-xs">
              Cihazınızın saat dilimi: {{ deviceTimeZone() }}. Onaydan sonra zaman değiştirilemez.
              Kesin alıcılar şimdi, onay sırasında seçilir.
            </p>
          </div>
          <p
            v-if="scheduleError"
            id="send-time-error"
            role="alert"
            class="text-destructive text-sm"
          >
            {{ scheduleError }}
          </p>
        </section>

        <Separator />

        <Button
          v-if="!confirming"
          type="button"
          size="lg"
          class="w-full"
          data-testid="approve-open"
          :disabled="disabled || pending"
          @click="openConfirmation"
        >
          <ShieldCheckIcon aria-hidden="true" />
          {{ pending ? 'Onaylanıyor…' : `Onayla (sürüm ${version})` }}
        </Button>

        <!-- Alert kendiliğinden role="alert" taşır; bu bir onay iletişimi olduğu için
           alertdialog rolüyle değiştirilir. -->
        <Alert v-else role="alertdialog" aria-labelledby="confirm-text">
          <AlertDescription class="text-foreground grid gap-4">
            <p id="confirm-text">
              Sürüm {{ version }} onaylanacak ve izinli alıcılar için gönderim işleri kuyruğa
              alınacak (önizleme: yaklaşık {{ audience.sms }} SMS, {{ audience.email }} e-posta;
              kesin sayı onay anındaki izin ve adres durumuna göre belirlenir). Onaydan sonra içerik
              değiştirilemez.
            </p>
            <div class="flex flex-col gap-3">
              <p v-if="confirmedTime" data-testid="confirm-schedule" class="text-sm font-medium">
                Gönderim başlangıcı: {{ formatScheduledTime(confirmedTime) }} ·
                {{ deviceTimeZone() }}. Mesajlar bu saatten önce gönderilmez; tamamlanma süresi hız
                sınırlarına bağlıdır.
              </p>
              <p v-else class="text-sm">Son onayınızdan sonra gönderim başlatılacak.</p>
              <Button
                type="button"
                size="lg"
                data-testid="approve-confirm"
                :disabled="pending"
                @click="confirm"
              >
                <CheckIcon aria-hidden="true" />
                Onayla ve kuyruğa al
              </Button>
              <Button
                type="button"
                variant="outline"
                size="lg"
                data-testid="approve-cancel"
                :disabled="pending"
                @click="confirming = false"
              >
                <XIcon aria-hidden="true" />
                Vazgeç
              </Button>
            </div>
          </AlertDescription>
        </Alert>

        <p v-if="disabled && !pending" class="text-muted-foreground text-sm">
          Onaylamak için önce değişiklikleri kaydedin; yalnızca kaydedilmiş son sürüm onaylanabilir.
        </p>
      </CardContent>
    </Card>
  </div>
</template>
