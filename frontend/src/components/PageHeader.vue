<script setup lang="ts">
import type { Component } from 'vue'
// Ekranların ortak üst bölümü: isteğe bağlı sayfa ikonu, sayfa başlığı, kampanya durum rozeti, kısa
// açıklama, başlığı içerikten ayıran ince degrade çizgi (Separator) ve altta adım göstergesi
// (varsayılan slot). İkon, kenar çubuğundaki menü öğesiyle aynıdır: aynı sayfa her yerde aynı
// simgeyle anılır. Düz, hafif tonlu bir karodur; yalnızca süstür (ekran okuyucudan gizli).
// Geri ve ileri gezinti `StepNav` bileşenindedir.
import { Separator } from '@/components/ui/separator'

defineProps<{
  title: string
  description?: string
  /** Sayfanın ikonu (lucide bileşeni); verilmezse ikon karosu çizilmez. */
  icon?: Component
}>()
</script>

<template>
  <header class="grid gap-4">
    <div class="flex items-start gap-3.5">
      <span
        v-if="icon"
        class="border-primary/20 bg-primary/10 text-primary grid size-11 shrink-0 place-items-center rounded-xl border"
        aria-hidden="true"
        data-testid="page-header-icon"
      >
        <component :is="icon" class="size-5" :stroke-width="1.75" />
      </span>
      <div class="grid min-w-0 gap-1">
        <div class="flex flex-wrap items-center gap-3">
          <h1 class="text-2xl font-semibold tracking-tight">{{ title }}</h1>
          <slot name="badge" />
        </div>
        <p v-if="description" class="text-muted-foreground text-sm" data-testid="page-description">
          {{ description }}
        </p>
      </div>
    </div>
    <Separator
      class="from-primary/60 via-border bg-linear-to-r to-transparent"
      data-testid="page-header-separator"
    />
    <slot />
  </header>
</template>
