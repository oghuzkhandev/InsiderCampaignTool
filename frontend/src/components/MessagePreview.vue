<script setup lang="ts">
import type { ContentFields } from '@/api/content'
import { NAME_VARIABLE, SAMPLE_NAME, renderPreview } from '@/content/draft'

// Mesajların alıcıya nasıl görüneceğinin ÖRNEĞİ: {{first_name}} yerine örnek bir ad konur.
// Yalnızca metin olarak basılır (HTML yorumlanmaz). Kesin içerik her alıcı için onayda
// ve gönderimde alıcının gerçek adıyla üretilir.
withDefaults(
  defineProps<{
    fields: ContentFields
    /** true ise (dar yan sütun) e-posta gövdesi açılır bir bölümde durur. */
    compact?: boolean
  }>(),
  { compact: false },
)
</script>

<template>
  <div class="grid gap-3" data-testid="message-preview">
    <p class="text-muted-foreground text-xs" data-testid="preview-note">
      Örnek önizleme: {{ NAME_VARIABLE }} yerine örnek ad olan "{{ SAMPLE_NAME }}" yazıldı. Gerçek
      mesajlar her alıcının kendi adıyla gönderilir.
    </p>

    <section class="grid gap-1" aria-label="SMS önizlemesi">
      <p class="text-xs font-medium">SMS</p>
      <p
        class="bg-muted rounded-lg border p-2 text-sm wrap-anywhere whitespace-pre-wrap"
        data-testid="preview-sms"
      >
        {{ renderPreview(fields.sms_text) || '—' }}
      </p>
    </section>

    <section class="grid gap-1" aria-label="E-posta önizlemesi">
      <p class="text-xs font-medium">E-posta</p>
      <div class="bg-muted grid gap-1 rounded-lg border p-2 text-sm">
        <p class="font-medium wrap-anywhere" data-testid="preview-subject">
          {{ renderPreview(fields.email_subject) || '—' }}
        </p>
        <details v-if="compact">
          <summary class="text-muted-foreground text-xs">E-posta gövdesini göster</summary>
          <p class="mt-1 wrap-anywhere whitespace-pre-wrap" data-testid="preview-body">
            {{ renderPreview(fields.email_body) || '—' }}
          </p>
        </details>
        <p v-else class="wrap-anywhere whitespace-pre-wrap" data-testid="preview-body">
          {{ renderPreview(fields.email_body) || '—' }}
        </p>
      </div>
    </section>
  </div>
</template>
