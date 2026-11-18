<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Audience } from '@/api/campaigns'

const props = defineProps<{
  /** Onaylanacak içerik sürümü. */
  version: number
  /** Önizlemedeki kitle sayıları; kesin sayı onay anında belirlenir. */
  audience: Audience
  /** Onay şu an yapılamaz (kaydedilmemiş değişiklik, işlem sürüyor). */
  disabled: boolean
  pending: boolean
}>()
const emit = defineEmits<{ approve: [] }>()

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
  <section class="approve" aria-labelledby="approve-title">
    <h2 id="approve-title">Onay</h2>
    <button
      v-if="!confirming"
      type="button"
      class="primary"
      :disabled="disabled || pending"
      @click="confirming = true"
    >
      Onayla (sürüm {{ version }})
    </button>
    <div v-else class="confirm" role="alertdialog" aria-labelledby="confirm-text">
      <p id="confirm-text">
        Sürüm {{ version }} onaylanacak ve izinli alıcılar için gönderim işleri kuyruğa alınacak
        (önizleme: yaklaşık {{ audience.sms }} SMS, {{ audience.email }} e-posta; kesin sayı onay
        anındaki izin ve adres durumuna göre belirlenir). Onaydan sonra içerik değiştirilemez.
      </p>
      <div class="actions">
        <button type="button" class="primary" :disabled="pending" @click="confirm">
          Onayla ve kuyruğa al
        </button>
        <button type="button" class="secondary" :disabled="pending" @click="confirming = false">
          Vazgeç
        </button>
      </div>
    </div>
    <p v-if="disabled && !pending" class="hint">
      Onaylamak için önce değişiklikleri kaydedin; yalnızca kaydedilmiş son sürüm onaylanabilir.
    </p>
  </section>
</template>

<style scoped>
.approve {
  display: grid;
  gap: 0.75rem;
}
h2 {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 700;
}
.confirm {
  display: grid;
  gap: 0.75rem;
  padding: 0.9rem 1rem;
  background: var(--app-warning-bg);
  border: 1px solid var(--app-border);
  border-radius: 8px;
}
.confirm p {
  margin: 0;
}
.actions {
  display: flex;
  gap: 0.6rem;
}
.primary,
.secondary {
  padding: 0.6rem 1.2rem;
  font: inherit;
  font-weight: 600;
  border-radius: 8px;
  cursor: pointer;
}
.primary {
  justify-self: start;
  color: var(--app-on-accent);
  background: var(--app-accent);
  border: none;
}
.secondary {
  color: inherit;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
}
button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.hint {
  margin: 0;
  color: var(--app-muted);
  font-size: 0.9rem;
}
</style>
