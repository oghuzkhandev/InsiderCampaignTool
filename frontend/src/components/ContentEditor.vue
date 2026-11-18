<script setup lang="ts">
import { computed } from 'vue'
import type { ContentFields } from '@/api/content'
import { MAX_SMS_LENGTH, MAX_SUBJECT_LENGTH, NAME_VARIABLE, estimatedLength } from '@/content/draft'

const fields = defineModel<ContentFields>({ required: true })
const props = defineProps<{
  /** true ise alanlar yalnızca okunur (akış sürerken veya onaydan sonra). */
  readonly: boolean
  /** Alan adına göre Türkçe doğrulama mesajları. */
  problems?: Record<string, string[]>
}>()

const smsLength = computed(() => estimatedLength(fields.value.sms_text))
const subjectLength = computed(() => estimatedLength(fields.value.email_subject))

function update(key: keyof ContentFields, value: string) {
  fields.value = { ...fields.value, [key]: value }
}

const problemsFor = (key: string) => props.problems?.[key] ?? []
</script>

<template>
  <div class="editor">
    <div class="field">
      <label for="sms_text">SMS metni</label>
      <textarea
        id="sms_text"
        rows="3"
        :value="fields.sms_text"
        :readonly="readonly"
        :aria-invalid="problemsFor('sms_text').length > 0 || smsLength > MAX_SMS_LENGTH"
        aria-describedby="sms-count sms-problems"
        @input="update('sms_text', ($event.target as HTMLTextAreaElement).value)"
      />
      <span id="sms-count" class="count" :class="{ over: smsLength > MAX_SMS_LENGTH }">
        {{ smsLength }} / {{ MAX_SMS_LENGTH }}
      </span>
      <ul id="sms-problems" class="problems" role="alert">
        <li v-for="(p, i) in problemsFor('sms_text')" :key="i">{{ p }}</li>
      </ul>
    </div>

    <div class="field">
      <label for="email_subject">E-posta konusu</label>
      <input
        id="email_subject"
        type="text"
        :value="fields.email_subject"
        :readonly="readonly"
        :aria-invalid="
          problemsFor('email_subject').length > 0 || subjectLength > MAX_SUBJECT_LENGTH
        "
        aria-describedby="subject-count subject-problems"
        @input="update('email_subject', ($event.target as HTMLInputElement).value)"
      />
      <span id="subject-count" class="count" :class="{ over: subjectLength > MAX_SUBJECT_LENGTH }">
        {{ subjectLength }} / {{ MAX_SUBJECT_LENGTH }}
      </span>
      <ul id="subject-problems" class="problems" role="alert">
        <li v-for="(p, i) in problemsFor('email_subject')" :key="i">{{ p }}</li>
      </ul>
    </div>

    <div class="field">
      <label for="email_body">E-posta gövdesi</label>
      <textarea
        id="email_body"
        rows="8"
        :value="fields.email_body"
        :readonly="readonly"
        :aria-invalid="problemsFor('email_body').length > 0"
        aria-describedby="body-problems"
        @input="update('email_body', ($event.target as HTMLTextAreaElement).value)"
      />
      <ul id="body-problems" class="problems" role="alert">
        <li v-for="(p, i) in problemsFor('email_body')" :key="i">{{ p }}</li>
      </ul>
    </div>

    <p class="hint">
      Alıcının adı için <code>{{ NAME_VARIABLE }}</code> kullanın; adı olmayan alıcıya "Değerli
      müşterimiz" yazılır. Sayaç bu varsayımla bir tahmindir, kesin uzunluk her alıcı için
      kaydederken ve onayda sunucuda doğrulanır.
    </p>
  </div>
</template>

<style scoped>
.editor {
  display: grid;
  gap: 1rem;
}
.field {
  display: grid;
  gap: 0.3rem;
}
label {
  font-weight: 600;
}
textarea,
input {
  width: 100%;
  box-sizing: border-box;
  padding: 0.65rem 0.75rem;
  font: inherit;
  color: inherit;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
}
textarea {
  resize: vertical;
}
textarea:read-only,
input:read-only {
  background: var(--app-bg);
}
textarea:focus-visible,
input:focus-visible {
  outline: 2px solid var(--app-accent);
  outline-offset: 1px;
}
[aria-invalid='true'] {
  border-color: var(--app-danger);
}
.count {
  justify-self: end;
  font-size: 0.85rem;
  color: var(--app-muted);
}
.count.over {
  color: var(--app-danger);
  font-weight: 600;
}
.problems {
  margin: 0;
  padding-left: 1.25rem;
  list-style: disc;
  color: var(--app-danger);
  font-size: 0.9rem;
}
.problems:empty {
  display: none;
}
.hint {
  margin: 0;
  color: var(--app-muted);
  font-size: 0.85rem;
}
</style>
