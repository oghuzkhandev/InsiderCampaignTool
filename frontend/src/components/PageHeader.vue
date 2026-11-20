<script setup lang="ts">
// Üç ekranın ortak üst bölümü: sayfa başlığı, kampanya durum rozeti, kısa açıklama ve altta
// adım göstergesi (varsayılan slot). Geri ve ileri gezinti `StepNav` bileşenindedir.
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <header class="grid gap-4">
    <div class="grid gap-1">
      <div class="flex flex-wrap items-center gap-3">
        <h1 class="text-2xl font-semibold tracking-tight">{{ title }}</h1>
        <slot name="badge" />
      </div>
      <p v-if="description" class="text-muted-foreground text-sm" data-testid="page-description">
        {{ description }}
      </p>
    </div>
    <slot />
  </header>
</template>
