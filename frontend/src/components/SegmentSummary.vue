<script setup lang="ts">
import { computed } from 'vue'
import type { Segment } from '@/api/campaigns'
import { describeSegment } from '@/segment/describe'

const props = defineProps<{ segment: Segment }>()
const lines = computed(() => describeSegment(props.segment))
</script>

<template>
  <section aria-labelledby="segment-title">
    <h2 id="segment-title">Anlaşılan koşullar</h2>
    <p class="hint">Hepsi birlikte geçerlidir. Beklediğiniz gibi değilse brief'i düzenleyin.</p>
    <ul>
      <li v-for="(line, i) in lines" :key="i">{{ line }}</li>
    </ul>
  </section>
</template>

<style scoped>
h2 {
  margin: 0 0 0.25rem;
  font-size: 1.1rem;
  font-weight: 700;
}
.hint {
  margin: 0 0 0.5rem;
  color: var(--app-muted);
  font-size: 0.9rem;
}
ul {
  margin: 0;
  padding-left: 1.25rem;
  list-style: disc;
}
</style>
