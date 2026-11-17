<script setup lang="ts">
defineProps<{ title: string; reasons: string[]; retryable: boolean }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="error-panel" role="alert">
    <p class="title">{{ title }}</p>
    <ul v-if="reasons.length > 0">
      <li v-for="(reason, i) in reasons" :key="i">{{ reason }}</li>
    </ul>
    <p v-if="reasons.length > 0" class="hint">Brief'i düzenleyip yeniden deneyebilirsiniz.</p>
    <button v-if="retryable" type="button" class="retry" @click="$emit('retry')">
      Yeniden dene
    </button>
  </div>
</template>

<style scoped>
.error-panel {
  padding: 0.9rem 1rem;
  background: var(--app-danger-bg);
  border: 1px solid var(--app-danger);
  border-radius: 8px;
}
.title {
  margin: 0;
  font-weight: 600;
}
ul {
  margin: 0.5rem 0 0;
  padding-left: 1.25rem;
  list-style: disc;
}
.hint {
  margin: 0.5rem 0 0;
  color: var(--app-muted);
  font-size: 0.9rem;
}
.retry {
  margin-top: 0.6rem;
  padding: 0.4rem 0.9rem;
  font: inherit;
  color: inherit;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  cursor: pointer;
}
</style>
