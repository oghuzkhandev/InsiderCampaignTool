<script setup lang="ts">
import { computed } from 'vue'
import {
  BanIcon,
  CircleCheckBigIcon,
  CircleXIcon,
  ClockIcon,
  RefreshCwIcon,
  SendIcon,
} from '@lucide/vue'
import type { MessageStatus } from '@/api/recipients'
import { messageStatusLabel } from '@/delivery/recipients'
import { Badge } from '@/components/ui/badge'

// Bir mesajın durumu: ikon + metin + yumuşak renk. Anlam yalnızca renkle değil, ikon ve metinle de
// verilir. Renkler StatusBadge ile aynıdır (açık ve koyu temada metin kontrastı yeterlidir).
const props = defineProps<{ status: MessageStatus; attempts: number }>()

const retrying = computed(() => props.status === 'queued' && props.attempts > 0)

const look = computed(() => {
  switch (props.status) {
    case 'queued':
      return retrying.value
        ? {
            icon: RefreshCwIcon,
            tone: 'bg-amber-100 text-amber-900 dark:bg-amber-400/15 dark:text-amber-300',
          }
        : {
            icon: ClockIcon,
            tone: 'bg-slate-100 text-slate-700 dark:bg-slate-400/15 dark:text-slate-300',
          }
    case 'sending':
      return {
        icon: SendIcon,
        tone: 'bg-sky-100 text-sky-900 dark:bg-sky-400/15 dark:text-sky-300',
      }
    case 'sent':
      return {
        icon: CircleCheckBigIcon,
        tone: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-400/15 dark:text-emerald-300',
      }
    case 'failed':
      return {
        icon: CircleXIcon,
        tone: 'bg-rose-100 text-rose-900 dark:bg-rose-400/15 dark:text-rose-300',
      }
    default:
      return {
        icon: BanIcon,
        tone: 'bg-slate-100 text-slate-700 dark:bg-slate-400/15 dark:text-slate-300',
      }
  }
})
</script>

<template>
  <Badge variant="outline" :class="['border-transparent', look.tone]" :data-status="status">
    <component :is="look.icon" aria-hidden="true" />
    {{ messageStatusLabel(status, attempts) }}
  </Badge>
</template>
