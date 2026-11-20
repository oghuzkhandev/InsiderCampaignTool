<script setup lang="ts">
import { computed } from 'vue'
import { ListFilterIcon } from '@lucide/vue'
import type { Segment } from '@/api/campaigns'
import { describeSegment } from '@/segment/describe'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

const props = defineProps<{ segment: Segment }>()
const lines = computed(() => describeSegment(props.segment))
</script>

<template>
  <Card role="region" aria-labelledby="segment-title">
    <CardHeader>
      <CardTitle class="flex items-center gap-2">
        <ListFilterIcon class="text-primary size-5" aria-hidden="true" />
        <h2 id="segment-title">Anlaşılan koşullar</h2>
      </CardTitle>
      <CardDescription>
        Hepsi birlikte geçerlidir. Beklediğiniz gibi değilse brief'i düzenleyin.
      </CardDescription>
    </CardHeader>
    <CardContent>
      <ul class="list-disc space-y-1 pl-5 text-sm">
        <li v-for="(line, i) in lines" :key="i">{{ line }}</li>
      </ul>
    </CardContent>
  </Card>
</template>
