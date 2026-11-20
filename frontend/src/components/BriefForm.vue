<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { LightbulbIcon, UsersIcon } from '@lucide/vue'
import { MAX_BRIEF_LENGTH } from '@/api/campaigns'
import { EXAMPLE_BRIEFS } from '@/exampleBrief'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

const brief = defineModel<string>({ required: true })
defineProps<{ loading: boolean }>()
const emit = defineEmits<{ submit: [brief: string] }>()

const showEmptyError = ref(false)
// Backend Unicode karakter sayar (rune); UTF-16 uzunluğu değil.
const length = computed(() => Array.from(brief.value).length)
const tooLong = computed(() => length.value > MAX_BRIEF_LENGTH)

watch(brief, () => {
  showEmptyError.value = false
})

function onSubmit() {
  const trimmed = brief.value.trim()
  if (!trimmed) {
    showEmptyError.value = true
    return
  }
  if (tooLong.value) return
  emit('submit', trimmed)
}

// Örnekler yalnızca alan boşken önerilir: kullanıcı yazmaya başlayınca yol göstermeye gerek
// kalmaz ve yazdığı metnin üzerine yazılmaz.
const showExamples = computed(() => brief.value.trim() === '')

function useExample(text: string) {
  brief.value = text
  // Seçilen örnek hemen düzenlenebilsin diye odak alana verilir.
  document.getElementById('brief')?.focus()
}
</script>

<template>
  <form class="grid gap-4" novalidate @submit.prevent="onSubmit">
    <Field>
      <FieldLabel for="brief">Brief</FieldLabel>
      <FieldDescription id="brief-hint">
        Kampanyanın kime gideceğini doğal dilde yazın. Örnek: şehir, son sepet ve satın alma
        tarihleri, sipariş sayısı.
      </FieldDescription>
      <Textarea
        id="brief"
        v-model="brief"
        class="min-h-32 wrap-anywhere"
        aria-describedby="brief-hint brief-count"
        :aria-invalid="showEmptyError || tooLong"
        @keydown.ctrl.enter.prevent="onSubmit"
        @keydown.meta.enter.prevent="onSubmit"
      />
      <div class="flex items-center justify-between gap-2 text-sm">
        <span
          id="brief-count"
          class="text-muted-foreground tabular-nums"
          :class="{ 'text-destructive font-semibold': tooLong }"
        >
          {{ length }} / {{ MAX_BRIEF_LENGTH }}
        </span>
      </div>
      <FieldError v-if="showEmptyError">Brief boş olamaz.</FieldError>
      <FieldError v-else-if="tooLong">
        Brief en fazla {{ MAX_BRIEF_LENGTH }} karakter olabilir.
      </FieldError>
    </Field>

    <div v-if="showExamples" class="grid gap-2" data-testid="examples">
      <p id="examples-label" class="text-muted-foreground flex items-start gap-2 text-sm">
        <LightbulbIcon
          class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-300"
          aria-hidden="true"
        />
        Nereden başlayacağınızı bilmiyorsanız bir örnek seçin, sonra istediğiniz gibi düzenleyin:
      </p>
      <ul class="grid gap-2" aria-labelledby="examples-label">
        <li v-for="example in EXAMPLE_BRIEFS" :key="example.id">
          <Button
            type="button"
            variant="outline"
            class="h-auto w-full flex-col items-start gap-0.5 py-2 text-left whitespace-normal"
            data-testid="example-brief"
            @click="useExample(example.brief)"
          >
            <span class="font-medium">{{ example.title }}</span>
            <span class="text-muted-foreground line-clamp-2 text-xs font-normal wrap-anywhere">
              {{ example.brief }}
            </span>
          </Button>
        </li>
      </ul>
    </div>

    <Button
      type="submit"
      size="lg"
      class="w-full sm:w-auto sm:justify-self-start"
      :disabled="loading || tooLong"
    >
      <UsersIcon aria-hidden="true" />
      {{ loading ? 'Hesaplanıyor…' : 'Kitleyi hesapla' }}
    </Button>
  </form>
</template>
