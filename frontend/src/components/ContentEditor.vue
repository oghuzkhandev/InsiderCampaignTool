<script setup lang="ts">
import { computed } from 'vue'
import type { ContentFields } from '@/api/content'
import { MAX_SMS_LENGTH, MAX_SUBJECT_LENGTH, NAME_VARIABLE, estimatedLength } from '@/content/draft'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

const fields = defineModel<ContentFields>({ required: true })
const props = defineProps<{
  /** true ise alanlar yalnızca okunur (akış sürerken veya onaydan sonra). */
  readonly: boolean
  /** Alan adına göre Türkçe doğrulama mesajları. */
  problems?: Record<string, string[]>
}>()

const smsLength = computed(() => estimatedLength(fields.value.sms_text))
const subjectLength = computed(() => estimatedLength(fields.value.email_subject))

function update(key: keyof ContentFields, value: string | number) {
  fields.value = { ...fields.value, [key]: String(value) }
}

const problemsFor = (key: string) => props.problems?.[key] ?? []
</script>

<template>
  <div class="grid gap-6">
    <Field>
      <div class="flex items-baseline justify-between gap-2">
        <FieldLabel for="sms_text">SMS metni</FieldLabel>
        <span
          id="sms-count"
          class="text-muted-foreground text-sm tabular-nums"
          :class="{ 'text-destructive font-semibold': smsLength > MAX_SMS_LENGTH }"
        >
          {{ smsLength }} / {{ MAX_SMS_LENGTH }}
        </span>
      </div>
      <Textarea
        id="sms_text"
        class="read-only:bg-muted min-h-20 wrap-anywhere"
        :model-value="fields.sms_text"
        :readonly="readonly"
        :aria-invalid="problemsFor('sms_text').length > 0 || smsLength > MAX_SMS_LENGTH"
        aria-describedby="sms-count sms-problems"
        @update:model-value="update('sms_text', $event)"
      />
      <ul
        id="sms-problems"
        class="text-destructive ml-4 list-disc text-sm empty:hidden"
        role="alert"
      >
        <li v-for="(p, i) in problemsFor('sms_text')" :key="i">{{ p }}</li>
      </ul>
    </Field>

    <Field>
      <div class="flex items-baseline justify-between gap-2">
        <FieldLabel for="email_subject">E-posta konusu</FieldLabel>
        <span
          id="subject-count"
          class="text-muted-foreground text-sm tabular-nums"
          :class="{ 'text-destructive font-semibold': subjectLength > MAX_SUBJECT_LENGTH }"
        >
          {{ subjectLength }} / {{ MAX_SUBJECT_LENGTH }}
        </span>
      </div>
      <Input
        id="email_subject"
        type="text"
        class="read-only:bg-muted"
        :model-value="fields.email_subject"
        :readonly="readonly"
        :aria-invalid="
          problemsFor('email_subject').length > 0 || subjectLength > MAX_SUBJECT_LENGTH
        "
        aria-describedby="subject-count subject-problems"
        @update:model-value="update('email_subject', $event)"
      />
      <ul
        id="subject-problems"
        class="text-destructive ml-4 list-disc text-sm empty:hidden"
        role="alert"
      >
        <li v-for="(p, i) in problemsFor('email_subject')" :key="i">{{ p }}</li>
      </ul>
    </Field>

    <Field>
      <FieldLabel for="email_body">E-posta gövdesi</FieldLabel>
      <Textarea
        id="email_body"
        class="read-only:bg-muted min-h-40 wrap-anywhere"
        :model-value="fields.email_body"
        :readonly="readonly"
        :aria-invalid="problemsFor('email_body').length > 0"
        aria-describedby="body-problems"
        @update:model-value="update('email_body', $event)"
      />
      <ul
        id="body-problems"
        class="text-destructive ml-4 list-disc text-sm empty:hidden"
        role="alert"
      >
        <li v-for="(p, i) in problemsFor('email_body')" :key="i">{{ p }}</li>
      </ul>
    </Field>

    <FieldDescription>
      Alıcının adı için <code class="bg-muted rounded px-1 font-mono">{{ NAME_VARIABLE }}</code>
      kullanın; adı olmayan alıcıya "Değerli müşterimiz" yazılır. Sayaç bu varsayımla bir tahmindir,
      kesin uzunluk her alıcı için kaydederken ve onayda sunucuda doğrulanır.
    </FieldDescription>
  </div>
</template>
