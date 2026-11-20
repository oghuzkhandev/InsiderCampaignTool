<script setup lang="ts">
import { MailIcon, MessageSquareIcon } from '@lucide/vue'
import type { ContentFields } from '@/api/content'
import { NAME_VARIABLE, SAMPLE_NAME, renderPreview } from '@/content/draft'

// Mesajların alıcıya nasıl görüneceğinin ÖRNEĞİ: {{first_name}} yerine örnek bir ad konur.
// Yalnızca metin olarak basılır (HTML yorumlanmaz). Kesin içerik her alıcı için onayda
// ve gönderimde alıcının gerçek adıyla üretilir. SMS ve e-posta her zaman açıktır (e-posta gövdesi
// de dahil); kapsayıcı yeterince genişse yan yana, dar ise alt alta durur (container query).
// Her mesaj, kanalını gösteren bir başlık şeridi ve içerikten çizgiyle ayrılmış bir karttır.
defineProps<{ fields: ContentFields }>()

const headerClass = 'bg-muted/50 flex items-center gap-1.5 border-b px-3 py-2 text-xs font-medium'
// Alan etiketi (Konu, Gövde): SMS ve e-posta aynı biçimi paylaşır.
const labelClass = 'text-muted-foreground text-xs font-medium'
</script>

<template>
  <div class="@container grid gap-3" data-testid="message-preview">
    <p class="text-muted-foreground text-xs" data-testid="preview-note">
      Örnek önizleme: {{ NAME_VARIABLE }} yerine örnek ad olan "{{ SAMPLE_NAME }}" yazıldı. Gerçek
      mesajlar her alıcının kendi adıyla gönderilir.
    </p>

    <div class="grid items-start gap-3 @[26rem]:grid-cols-2">
      <section class="overflow-hidden rounded-lg border" aria-label="SMS önizlemesi">
        <p :class="headerClass">
          <MessageSquareIcon class="text-muted-foreground size-3.5" aria-hidden="true" />
          SMS
        </p>
        <div class="grid gap-1 p-3 text-sm">
          <p :class="labelClass" data-testid="preview-sms-label">Gövde</p>
          <p class="wrap-anywhere whitespace-pre-wrap" data-testid="preview-sms">
            {{ renderPreview(fields.sms_text) || '—' }}
          </p>
        </div>
      </section>

      <section class="overflow-hidden rounded-lg border" aria-label="E-posta önizlemesi">
        <p :class="headerClass">
          <MailIcon class="text-muted-foreground size-3.5" aria-hidden="true" />
          E-posta
        </p>
        <div class="grid gap-3 p-3 text-sm">
          <div class="grid gap-1">
            <p :class="labelClass" data-testid="preview-subject-label">Konu</p>
            <p class="font-semibold wrap-anywhere" data-testid="preview-subject">
              {{ renderPreview(fields.email_subject) || '—' }}
            </p>
          </div>
          <div class="border-t" role="separator" />
          <div class="grid gap-1">
            <p :class="labelClass" data-testid="preview-body-label">Gövde</p>
            <p class="wrap-anywhere whitespace-pre-wrap" data-testid="preview-body">
              {{ renderPreview(fields.email_body) || '—' }}
            </p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
