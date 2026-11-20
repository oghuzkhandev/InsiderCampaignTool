<script setup lang="ts">
import PageHeader from '@/components/PageHeader.vue'
import AudienceDirectory from '@/components/AudienceDirectory.vue'
import { Card, CardContent } from '@/components/ui/card'
</script>
<template>
  <main class="grid gap-6">
    <PageHeader
      title="Kitleler"
      description="Demo kullanıcılarını arayın; şehre ve kanal uygunluğuna göre süzün. Kampanyaya özel eşleşmeler kitle önizlemesindedir."
    />
    <Card
      ><CardContent><AudienceDirectory /></CardContent
    ></Card>
  </main>
</template>
