<script setup lang="ts">
import { computed } from 'vue'
import {
  CircleCheckBigIcon,
  CircleHelpIcon,
  ClockIcon,
  PencilLineIcon,
  SendIcon,
} from '@lucide/vue'
import { campaignStatusLabel } from '@/delivery/messages'
import { Badge } from '@/components/ui/badge'

// Kampanya durumu: ikon + metin + yumuşak renk. Anlam yalnızca renkle değil, ikon ve metinle de
// verilir. Renkler hem açık hem koyu temada metin kontrastı 4,5'in üzerindedir (e2e'de ölçülür).
const props = defineProps<{ status: string }>()

const look = computed(() => {
  switch (props.status) {
    case 'draft':
      return {
        icon: PencilLineIcon,
        tone: 'bg-slate-100 text-slate-700 dark:bg-slate-400/15 dark:text-slate-300',
      }
    case 'approved':
      return {
        icon: ClockIcon,
        tone: 'bg-amber-100 text-amber-900 dark:bg-amber-400/15 dark:text-amber-300',
      }
    case 'sending':
      return {
        icon: SendIcon,
        tone: 'bg-sky-100 text-sky-900 dark:bg-sky-400/15 dark:text-sky-300',
      }
    case 'completed':
      return {
        icon: CircleCheckBigIcon,
        tone: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-400/15 dark:text-emerald-300',
      }
    default:
      return {
        icon: CircleHelpIcon,
        tone: 'bg-slate-100 text-slate-700 dark:bg-slate-400/15 dark:text-slate-300',
      }
  }
})
</script>

<template>
  <Badge variant="outline" :class="['border-transparent', look.tone]" :data-status="status">
    <component :is="look.icon" aria-hidden="true" />
    {{ campaignStatusLabel(status) }}
  </Badge>
</template>
