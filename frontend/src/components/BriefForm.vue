<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { EraserIcon, LightbulbIcon, UsersIcon } from '@lucide/vue'
import { MAX_BRIEF_LENGTH } from '@/api/campaigns'
import { EXAMPLE_BRIEFS } from '@/exampleBrief'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { Separator } from '@/components/ui/separator'

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

function clearBrief() {
  brief.value = ''
  showEmptyError.value = false
  document.getElementById('brief')?.focus()
}
</script>

<template>
  <form class="grid gap-8" novalidate @submit.prevent="onSubmit">
    <Card>
      <CardHeader class="gap-2">
        <CardTitle><h2>Kampanyanızı tanımlayın</h2></CardTitle>
        <CardDescription>
          Kimlere ulaşmak istediğinizi yazın. Ardından eşleşen kişileri ve kanal sayılarını
          hesaplayın.
        </CardDescription>
      </CardHeader>
      <CardContent class="grid gap-6">
        <Separator />
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
            <Button
              type="button"
              variant="ghost"
              size="sm"
              :disabled="loading || brief.length === 0"
              data-testid="clear-brief"
              @click="clearBrief"
            >
              <EraserIcon aria-hidden="true" />
              Temizle
            </Button>
          </div>
          <FieldError v-if="showEmptyError">Brief boş olamaz.</FieldError>
          <FieldError v-else-if="tooLong">
            Brief en fazla {{ MAX_BRIEF_LENGTH }} karakter olabilir.
          </FieldError>
        </Field>

        <Separator />
        <Button
          type="submit"
          size="lg"
          class="w-full sm:w-auto sm:justify-self-start"
          :disabled="loading || tooLong"
        >
          <UsersIcon aria-hidden="true" />
          {{ loading ? 'Hesaplanıyor…' : 'Kitleyi hesapla' }}
        </Button>
      </CardContent>
    </Card>

    <Card v-if="showExamples" data-testid="examples">
      <CardHeader class="gap-2">
        <CardTitle class="flex items-center gap-2">
          <LightbulbIcon
            class="size-5 shrink-0 text-amber-600 dark:text-amber-300"
            aria-hidden="true"
          />
          <h2 id="examples-label">Örneklerle başlayın</h2>
        </CardTitle>
        <CardDescription>
          Aşağıdaki örneklerden birini seçerek brief alanını doldurabilirsiniz. Metni kendi
          kampanyanıza göre düzenleyip kitleyi hesaplayın.
        </CardDescription>
      </CardHeader>
      <CardContent class="grid gap-5">
        <Separator />
        <ul class="grid gap-4" aria-labelledby="examples-label">
          <li v-for="example in EXAMPLE_BRIEFS" :key="example.id">
            <Button
              type="button"
              variant="outline"
              class="h-auto w-full flex-col items-start gap-3 px-4 py-4 text-left whitespace-normal"
              data-testid="example-brief"
              @click="useExample(example.brief)"
            >
              <span class="grid gap-1">
                <span class="text-muted-foreground text-xs font-normal">Örnek kampanya</span>
                <span class="font-semibold wrap-anywhere">{{ example.title }}</span>
              </span>
              <span class="bg-border h-px w-full" aria-hidden="true" />
              <span class="grid gap-1.5">
                <span class="text-muted-foreground text-xs font-medium">Brief metni</span>
                <span
                  class="text-muted-foreground text-sm leading-relaxed font-normal wrap-anywhere"
                >
                  {{ example.brief }}
                </span>
              </span>
              <span class="text-primary pt-1 text-xs font-medium">
                Bu örneği brief alanına aktar
              </span>
            </Button>
          </li>
        </ul>
      </CardContent>
    </Card>
  </form>
</template>
