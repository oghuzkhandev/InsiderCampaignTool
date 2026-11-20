<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  ClockIcon,
  FileStackIcon,
  LayoutTemplateIcon,
  MailIcon,
  MessageSquareIcon,
  PencilLineIcon,
  PlusIcon,
  SaveIcon,
  SquarePenIcon,
  TrashIcon,
} from '@lucide/vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { deleteTemplate, getTemplates, saveTemplate, type ContentTemplate } from '@/api/workspace'
import { useResource } from '@/composables/useResource'
import { notify } from '@/notify/notify'
import { emptyFields } from '@/content/draft'
import { contentReasonMessage } from '@/content/messages'
import { useUnsavedChangesGuard, UNSAVED_MESSAGE } from '@/content/useUnsavedChangesGuard'
import ContentEditor from '@/components/ContentEditor.vue'
import PageHeader from '@/components/PageHeader.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import ListPagination from '@/components/ListPagination.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
const page = ref(1)
const { data, loading, error, reload } = useResource((signal) => getTemplates(page.value, signal))
watch(page, reload, { immediate: true })
const editingId = ref<string | null>(null)
const name = ref('')
const fields = ref(emptyFields())
const pending = ref(false)
const saveError = ref<ApiError | null>(null)
const baseline = ref(JSON.stringify({ name: '', ...emptyFields() }))
const dirty = computed(
  () => baseline.value !== JSON.stringify({ name: name.value, ...fields.value }),
)
const problems = computed(() => {
  const result: Record<string, string[]> = {}
  for (const reason of saveError.value?.reasons ?? [])
    if (reason.field) (result[reason.field] ??= []).push(contentReasonMessage(reason))
  return result
})
let controller: AbortController | null = null
useUnsavedChangesGuard(() => dirty.value)
onBeforeUnmount(() => {
  controller?.abort()
  controller = null
})

const nf = new Intl.NumberFormat('tr-TR')
const dateFormat = new Intl.DateTimeFormat('tr-TR', { dateStyle: 'medium', timeStyle: 'short' })
/** Güncellenme zamanının yerel gösterimi ("20 Kas 2026 14:30"). */
const updatedText = (iso: string) => dateFormat.format(new Date(iso))

function edit(item?: ContentTemplate) {
  if (pending.value || (dirty.value && !window.confirm(UNSAVED_MESSAGE))) return
  editingId.value = item?.id ?? null
  name.value = item?.name ?? ''
  fields.value = item
    ? { sms_text: item.sms_text, email_subject: item.email_subject, email_body: item.email_body }
    : emptyFields()
  baseline.value = JSON.stringify({ name: name.value, ...fields.value })
  saveError.value = null
}
async function submit() {
  if (pending.value || !dirty.value) return
  pending.value = true
  saveError.value = null
  const current = new AbortController()
  controller = current
  try {
    const item = await saveTemplate(editingId.value, name.value, fields.value, current.signal)
    if (controller !== current) return
    editingId.value = item.id
    name.value = item.name
    fields.value = {
      sms_text: item.sms_text,
      email_subject: item.email_subject,
      email_body: item.email_body,
    }
    baseline.value = JSON.stringify({ name: name.value, ...fields.value })
    notify.success('Şablon kaydedildi', 'Kampanyanın içerik ekranında kullanabilirsiniz.')
    await reload()
  } catch (err) {
    if (controller === current && !isAbortError(err)) {
      saveError.value =
        err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Şablon kaydedilemedi.')
      notify.error('Şablon kaydedilemedi', saveError.value.message)
    }
  } finally {
    if (controller === current) pending.value = false
  }
}
async function remove(item: ContentTemplate) {
  if (
    pending.value ||
    !window.confirm(
      `“${item.name}” şablonu silinsin mi? Kaydedilmiş kampanya içerikleri etkilenmez.`,
    )
  )
    return
  if (editingId.value === item.id && dirty.value && !window.confirm(UNSAVED_MESSAGE)) return
  pending.value = true
  saveError.value = null
  const current = new AbortController()
  controller = current
  try {
    await deleteTemplate(item.id, current.signal)
    if (controller !== current) return
    if (editingId.value === item.id) {
      editingId.value = null
      name.value = ''
      fields.value = emptyFields()
      baseline.value = JSON.stringify({ name: '', ...emptyFields() })
    }
    notify.success(
      'Şablon silindi',
      `“${item.name}” silindi; kaydedilmiş kampanya içerikleri etkilenmedi.`,
    )
    if (data.value?.items.length === 1 && page.value > 1) page.value -= 1
    else await reload()
  } catch (err) {
    if (controller === current && !isAbortError(err)) {
      saveError.value =
        err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Şablon silinemedi.')
      notify.error('Şablon silinemedi', saveError.value.message)
    }
  } finally {
    if (controller === current) pending.value = false
  }
}
</script>
<template>
  <main class="grid gap-8">
    <PageHeader
      :icon="LayoutTemplateIcon"
      title="İçerik şablonları"
      description="Tekrar kullanacağınız SMS ve e-posta metinlerini hazırlayın. Şablonlar kendiliğinden gönderim başlatmaz."
    />
    <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
      <Card>
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <FileStackIcon class="text-primary size-5" aria-hidden="true" />
            <h2>Kayıtlı şablonlar</h2>
          </CardTitle>
          <CardDescription v-if="data && data.total > 0" data-testid="template-total">
            {{ nf.format(data.total) }} şablon
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4">
          <Separator />

          <div v-if="loading && !data" class="grid gap-3" role="status">
            <span class="sr-only">Şablonlar yükleniyor…</span>
            <Skeleton v-for="n in 3" :key="n" class="h-28 w-full" />
          </div>
          <ErrorPanel
            v-else-if="error"
            :title="error"
            :reasons="[]"
            :retryable="true"
            @retry="reload"
          />
          <Empty v-else-if="!data?.items.length" class="border" data-testid="template-empty">
            <EmptyHeader>
              <EmptyMedia variant="icon"><LayoutTemplateIcon aria-hidden="true" /></EmptyMedia>
              <EmptyTitle>Henüz şablon yok</EmptyTitle>
              <EmptyDescription>İlk şablonunuzu hazırlayın.</EmptyDescription>
            </EmptyHeader>
          </Empty>

          <ul
            v-else
            class="divide-y overflow-hidden rounded-xl border"
            :aria-busy="loading"
            :class="{ 'opacity-60': loading }"
          >
            <li
              v-for="item in data.items"
              :key="item.id"
              class="grid min-w-0 gap-3 p-4 transition-colors duration-200"
              :class="{ 'bg-primary/5': item.id === editingId }"
              :data-editing="item.id === editingId"
              data-testid="template-row"
            >
              <div class="flex items-start justify-between gap-3">
                <h3 class="min-w-0 text-sm leading-6 wrap-anywhere">
                  <span class="text-muted-foreground">{{ 'Şablon adı: ' }}</span>
                  <span class="font-semibold">{{ item.name }}</span>
                </h3>
                <Badge
                  v-if="item.id === editingId"
                  variant="outline"
                  class="border-primary/25 bg-primary/10 text-primary shrink-0 rounded-full px-2.5 py-1 text-xs font-medium"
                >
                  Düzenleme açık
                </Badge>
              </div>

              <dl class="divide-y rounded-lg border text-sm">
                <div class="grid grid-cols-[5rem_minmax(0,1fr)] items-baseline gap-3 px-3 py-2">
                  <dt class="text-muted-foreground flex items-center gap-1.5 text-xs">
                    <MessageSquareIcon class="size-3.5" aria-hidden="true" />
                    SMS
                  </dt>
                  <dd class="line-clamp-2 wrap-anywhere" data-testid="template-sms">
                    {{ item.sms_text }}
                  </dd>
                </div>
                <div class="grid grid-cols-[5rem_minmax(0,1fr)] items-baseline gap-3 px-3 py-2">
                  <dt class="text-muted-foreground flex items-center gap-1.5 text-xs">
                    <MailIcon class="size-3.5" aria-hidden="true" />
                    Konu
                  </dt>
                  <dd class="line-clamp-1 wrap-anywhere" data-testid="template-subject">
                    {{ item.email_subject }}
                  </dd>
                </div>
                <div class="grid min-w-0 gap-2 px-3 py-3">
                  <dt class="text-muted-foreground flex items-center gap-1.5 text-xs">
                    <MailIcon class="size-3.5 shrink-0" aria-hidden="true" />
                    E-posta gövdesi
                  </dt>
                  <dd
                    class="text-foreground text-sm leading-relaxed whitespace-pre-wrap wrap-anywhere"
                    data-testid="template-body"
                  >
                    {{ item.email_body }}
                  </dd>
                </div>
              </dl>

              <div class="flex flex-wrap items-center justify-between gap-2 border-t pt-3">
                <p
                  class="text-muted-foreground flex items-center gap-1.5 text-xs"
                  data-testid="template-updated"
                >
                  <ClockIcon class="size-3.5" aria-hidden="true" />
                  Güncellendi: {{ updatedText(item.updated_at) }}
                </p>
                <div class="flex gap-3">
                  <Button
                    variant="outline"
                    size="sm"
                    :disabled="pending"
                    @click="item.id === editingId ? edit() : edit(item)"
                  >
                    <PencilLineIcon v-if="item.id !== editingId" aria-hidden="true" />
                    {{ item.id === editingId ? 'Vazgeç' : 'Düzenle' }}
                    <span class="sr-only"> {{ item.name }}</span>
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    class="text-destructive hover:text-destructive"
                    :disabled="pending"
                    @click="remove(item)"
                  >
                    <TrashIcon aria-hidden="true" />
                    Sil<span class="sr-only"> {{ item.name }}</span>
                  </Button>
                </div>
              </div>
            </li>
          </ul>

          <ListPagination
            v-if="data"
            :page="page"
            :total="data.total"
            :limit="data.limit"
            :loading="loading || pending"
            @change="page = $event"
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <SquarePenIcon class="text-primary size-5" aria-hidden="true" />
            <h2>{{ editingId ? 'Şablonu düzenle' : 'Yeni şablon' }}</h2>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form class="grid gap-6" @submit.prevent="submit">
            <Separator />
            <div class="grid gap-2">
              <Label for="template-name">Şablon adı</Label>
              <Input
                id="template-name"
                v-model="name"
                maxlength="100"
                required
                :readonly="pending"
                placeholder="Örneğin: sepet hatırlatması"
              />
            </div>
            <Separator />
            <ContentEditor v-model="fields" :readonly="pending" :problems="problems" />
            <p class="text-muted-foreground text-xs">
              Şablon kampanyaya uygulandığında alıcı adlarının uzunluğuna göre yeniden doğrulanır.
              Uygulamak mevcut kampanyayı onaylamaz.
            </p>
            <ErrorPanel
              v-if="saveError"
              :title="saveError.message"
              :reasons="[]"
              :retryable="false"
            />
            <Separator />
            <div class="flex flex-wrap gap-3 pt-2">
              <Button type="submit" :disabled="pending || !dirty">
                <SaveIcon aria-hidden="true" />
                {{ pending ? 'Kaydediliyor…' : 'Şablonu kaydet' }}
              </Button>
              <Button type="button" variant="outline" :disabled="pending" @click="edit()">
                <PlusIcon v-if="!editingId" aria-hidden="true" />
                {{ editingId ? 'Düzenlemekten vazgeç' : 'Yeni şablon' }}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  </main>
</template>
