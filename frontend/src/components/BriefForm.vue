<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MAX_BRIEF_LENGTH } from '@/api/campaigns'
import { EXAMPLE_BRIEF } from '@/exampleBrief'

const brief = defineModel<string>({ required: true })
defineProps<{ loading: boolean }>()
const emit = defineEmits<{ submit: [brief: string] }>()

const showEmptyError = ref(false)
// Backend Unicode karakter sayar (rune); UTF-16 uzunluğu değil.
const length = computed(() => Array.from(brief.value).length)
const tooLong = computed(() => length.value > MAX_BRIEF_LENGTH)

watch(brief, () => {
  showEmptyError.value = false
})

function onSubmit() {
  const trimmed = brief.value.trim()
  if (!trimmed) {
    showEmptyError.value = true
    return
  }
  if (tooLong.value) return
  emit('submit', trimmed)
}

function useExample() {
  brief.value = EXAMPLE_BRIEF
}
</script>

<template>
  <form class="brief-form" novalidate @submit.prevent="onSubmit">
    <label for="brief">Brief</label>
    <p id="brief-hint" class="hint">
      Kampanyanın kime gideceğini doğal dilde yazın. Örnek: şehir, son sepet ve satın alma
      tarihleri, sipariş sayısı.
    </p>
    <textarea
      id="brief"
      v-model="brief"
      rows="5"
      aria-describedby="brief-hint brief-count"
      :aria-invalid="showEmptyError || tooLong"
      @keydown.ctrl.enter.prevent="onSubmit"
      @keydown.meta.enter.prevent="onSubmit"
    />
    <div class="meta">
      <span id="brief-count" :class="{ over: tooLong }">
        {{ length }} / {{ MAX_BRIEF_LENGTH }}
      </span>
      <button type="button" class="link" @click="useExample">Örnek brief'i kullan</button>
    </div>
    <p v-if="showEmptyError" class="field-error" role="alert">Brief boş olamaz.</p>
    <p v-else-if="tooLong" class="field-error" role="alert">
      Brief en fazla {{ MAX_BRIEF_LENGTH }} karakter olabilir.
    </p>
    <button type="submit" class="primary" :disabled="loading || tooLong">
      {{ loading ? 'Hesaplanıyor…' : 'Kitleyi hesapla' }}
    </button>
  </form>
</template>

<style scoped>
.brief-form {
  display: grid;
  gap: 0.5rem;
}
label {
  font-weight: 600;
}
.hint {
  margin: 0;
  color: var(--app-muted);
  font-size: 0.9rem;
}
textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 0.75rem;
  font: inherit;
  color: inherit;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  resize: vertical;
}
textarea:focus-visible {
  outline: 2px solid var(--app-accent);
  outline-offset: 1px;
}
textarea[aria-invalid='true'] {
  border-color: var(--app-danger);
}
.meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.85rem;
  color: var(--app-muted);
}
.over {
  color: var(--app-danger);
  font-weight: 600;
}
.field-error {
  margin: 0;
  color: var(--app-danger);
  font-size: 0.9rem;
}
.link {
  background: none;
  border: none;
  padding: 0;
  color: var(--app-accent);
  font: inherit;
  cursor: pointer;
  text-decoration: underline;
}
.primary {
  justify-self: start;
  padding: 0.6rem 1.2rem;
  font: inherit;
  font-weight: 600;
  color: var(--app-on-accent);
  background: var(--app-accent);
  border: none;
  border-radius: 8px;
  cursor: pointer;
}
.primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
