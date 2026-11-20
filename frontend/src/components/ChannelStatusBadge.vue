<script setup lang="ts">
import { computed } from 'vue'
import { BanIcon, CircleCheckIcon, CircleSlashIcon } from '@lucide/vue'
import type { ChannelStatus } from '@/api/audience'
import { channelStatusHint, channelStatusLabel, type Channel } from '@/audience/members'
import { Badge } from '@/components/ui/badge'

// Bir kişinin bir kanala uygunluğu: ikon + metin + yumuşak renk. Anlam yalnızca renkle değil, ikon
// ve metinle de verilir. Renkler StatusBadge ile aynıdır (açık ve koyu temada kontrast ölçülür).
const props = defineProps<{ status: ChannelStatus; channel: Channel }>()

const look = computed(() => {
  switch (props.status) {
    case 'eligible':
      return {
        icon: CircleCheckIcon,
        tone: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-400/15 dark:text-emerald-300',
      }
    case 'no_consent':
      return {
        icon: BanIcon,
        tone: 'bg-amber-100 text-amber-900 dark:bg-amber-400/15 dark:text-amber-300',
      }
    default:
      return {
        icon: CircleSlashIcon,
        tone: 'bg-slate-100 text-slate-700 dark:bg-slate-400/15 dark:text-slate-300',
      }
  }
})
</script>

<template>
  <Badge
    variant="outline"
    :class="['border-transparent', look.tone]"
    :data-status="status"
    :title="channelStatusHint(channel, status)"
  >
    <component :is="look.icon" aria-hidden="true" />
    {{ channelStatusLabel(status) }}
  </Badge>
</template>
