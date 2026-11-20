<script setup lang="ts">
import { computed } from 'vue'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// Süzgeç seçici (shadcn `Select`): boş değer "süzme yok" demektir ve ilk seçenek olarak `allLabel`
// ile gösterilir. shadcn `Select` boş metin değerini kabul etmediği için "tümü" içeride bir işaretçi
// değerle (ALL) temsil edilir; dışarıya her zaman '' olarak çıkar. Süzgeç etkinken tetikleyici vurgulanır.
// `id` ve diğer öznitelikler (ör. data-testid) tetikleyiciye gider; böylece <Label for> çalışır.
defineOptions({ inheritAttrs: false })

const props = defineProps<{
  id: string
  modelValue: string
  options: { value: string; label: string }[]
  /** "Süzme yok" seçeneğinin ve boşken tetikleyicide görünen metin (ör. "Tüm şehirler"). */
  allLabel: string
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const ALL = '__all__'
const selected = computed({
  get: () => props.modelValue || ALL,
  set: (value: string) => emit('update:modelValue', value === ALL ? '' : value),
})
</script>

<template>
  <Select v-model="selected" :disabled="disabled">
    <SelectTrigger
      :id="id"
      v-bind="$attrs"
      :data-active="modelValue !== ''"
      class="data-[active=true]:border-primary/60 data-[active=true]:bg-primary/5 h-10 w-full rounded-lg duration-200"
    >
      <SelectValue :placeholder="allLabel" />
    </SelectTrigger>
    <SelectContent
      position="popper"
      :side-offset="6"
      class="bg-popover max-h-72 rounded-lg border shadow-lg"
    >
      <SelectItem :value="ALL" class="py-2">{{ allLabel }}</SelectItem>
      <SelectSeparator />
      <SelectItem v-for="option in options" :key="option.value" :value="option.value" class="py-2">
        {{ option.label }}
      </SelectItem>
    </SelectContent>
  </Select>
</template>
