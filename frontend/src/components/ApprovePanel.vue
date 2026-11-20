<script setup lang="ts">
import { ref, watch } from 'vue'
import { CheckIcon, ShieldCheckIcon, XIcon } from '@lucide/vue'
import type { Audience } from '@/api/campaigns'
import type { ContentFields } from '@/api/content'
import MessagePreview from '@/components/MessagePreview.vue'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

const props = defineProps<{
  /** Onaylanacak içerik sürümü. */
  version: number
  /** Onaylanacak (kaydedilmiş) sürümün metinleri; önizleme bunlardan üretilir. */
  content: ContentFields
  /** Önizlemedeki kitle sayıları; kesin sayı onay anında belirlenir. */
  audience: Audience
  /** Onay şu an yapılamaz (kaydedilmemiş değişiklik, işlem sürüyor). */
  disabled: boolean
  pending: boolean
}>()
const emit = defineEmits<{ approve: [] }>()

const nf = new Intl.NumberFormat('tr-TR')
const confirming = ref(false)

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
  confirming.value = false
  emit('approve')
}
</script>

<template>
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

    <CardContent class="grid gap-4">
      <!-- Onaydan önce görülmesi gerekenler aynı yerde: hangi sürüm, kimlere ve ne gidecek. -->
      <dl class="grid gap-1 text-sm" data-testid="approve-recipients">
        <div class="flex items-baseline justify-between gap-3">
          <dt class="text-muted-foreground">Tahmini SMS alıcısı</dt>
          <dd class="font-bold tabular-nums" data-testid="approve-sms">
            ≈ {{ nf.format(audience.sms) }}
          </dd>
        </div>
        <div class="flex items-baseline justify-between gap-3">
          <dt class="text-muted-foreground">Tahmini e-posta alıcısı</dt>
          <dd class="font-bold tabular-nums" data-testid="approve-email">
            ≈ {{ nf.format(audience.email) }}
          </dd>
        </div>
      </dl>
      <p class="text-muted-foreground -mt-2 text-xs">
        Sayılar önizlemeye göredir; kesin sayı onay anındaki izin ve adres durumuna göre belirlenir.
      </p>

      <MessagePreview compact :fields="content" />

      <Button
        v-if="!confirming"
        type="button"
        size="lg"
        class="w-full"
        data-testid="approve-open"
        :disabled="disabled || pending"
        @click="confirming = true"
      >
        <ShieldCheckIcon aria-hidden="true" />
        {{ pending ? 'Onaylanıyor…' : `Onayla (sürüm ${version})` }}
      </Button>

      <!-- Alert kendiliğinden role="alert" taşır; bu bir onay iletişimi olduğu için
           alertdialog rolüyle değiştirilir. -->
      <Alert v-else role="alertdialog" aria-labelledby="confirm-text">
        <AlertDescription class="text-foreground grid gap-4">
          <p id="confirm-text">
            Sürüm {{ version }} onaylanacak ve izinli alıcılar için gönderim işleri kuyruğa alınacak
            (önizleme: yaklaşık {{ audience.sms }} SMS, {{ audience.email }} e-posta; kesin sayı
            onay anındaki izin ve adres durumuna göre belirlenir). Onaydan sonra içerik
            değiştirilemez.
          </p>
          <div class="flex flex-col gap-2">
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
</template>
