<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="page">
    <RouterView />
  </div>
</template>

<style scoped>
.page {
  /* preflight border-box: 48rem içerik + 2rem yatay padding */
  max-width: calc(48rem + 2rem);
  margin: 0 auto;
  padding: 2rem 1rem 4rem;
}
</style>
