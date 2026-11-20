<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { deleteTemplate, getTemplates, saveTemplate, type ContentTemplate } from '@/api/workspace'
import { useResource } from '@/composables/useResource'
import { emptyFields } from '@/content/draft'
import { contentReasonMessage } from '@/content/messages'
import { useUnsavedChangesGuard, UNSAVED_MESSAGE } from '@/content/useUnsavedChangesGuard'
import ContentEditor from '@/components/ContentEditor.vue'
import PageHeader from '@/components/PageHeader.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import ListPagination from '@/components/ListPagination.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
const page = ref(1)
const { data, loading, error, reload } = useResource((signal) => getTemplates(page.value, signal))
watch(page, reload, { immediate: true })
const editingId = ref<string | null>(null)
const name = ref('')
const fields = ref(emptyFields())
const pending = ref(false)
const saveError = ref<ApiError | null>(null)
const feedback = ref('')
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

function edit(item?: ContentTemplate) {
  if (pending.value || (dirty.value && !window.confirm(UNSAVED_MESSAGE))) return
  editingId.value = item?.id ?? null
  name.value = item?.name ?? ''
  fields.value = item
    ? { sms_text: item.sms_text, email_subject: item.email_subject, email_body: item.email_body }
    : emptyFields()
  baseline.value = JSON.stringify({ name: name.value, ...fields.value })
  saveError.value = null
  feedback.value = ''
}
async function submit() {
  if (pending.value || !dirty.value) return
  pending.value = true
  saveError.value = null
  feedback.value = ''
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
    feedback.value = 'Şablon kaydedildi. Kampanyanın içerik ekranında kullanabilirsiniz.'
    await reload()
  } catch (err) {
    if (controller === current && !isAbortError(err))
      saveError.value =
        err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Şablon kaydedilemedi.')
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
    feedback.value = 'Şablon silindi.'
    if (data.value?.items.length === 1 && page.value > 1) page.value -= 1
    else await reload()
  } catch (err) {
    if (controller === current && !isAbortError(err))
      saveError.value =
        err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Şablon silinemedi.')
  } finally {
    if (controller === current) pending.value = false
  }
}
</script>
<template>
  <main class="grid gap-6">
    <PageHeader
      title="İçerik şablonları"
      description="Tekrar kullanacağınız SMS ve e-posta metinlerini hazırlayın. Şablonlar kendiliğinden gönderim başlatmaz."
    />
    <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
      <Card
        ><CardHeader
          ><CardTitle><h2>Kayıtlı şablonlar</h2></CardTitle></CardHeader
        ><CardContent class="grid gap-4">
          <p v-if="loading" role="status" class="text-muted-foreground text-sm">
            Şablonlar yükleniyor…
          </p>
          <ErrorPanel
            v-else-if="error"
            :title="error"
            :reasons="[]"
            :retryable="true"
            @retry="reload"
          />
          <p v-else-if="!data?.items.length" class="text-muted-foreground text-sm">
            Henüz şablon yok. İlk şablonunuzu hazırlayın.
          </p>
          <ul v-else class="grid gap-3">
            <li
              v-for="item in data.items"
              :key="item.id"
              class="grid min-w-0 gap-3 rounded-lg border p-3"
              data-testid="template-row"
            >
              <h3 class="font-medium wrap-anywhere">{{ item.name }}</h3>
              <p class="text-muted-foreground line-clamp-2 text-sm wrap-anywhere">
                {{ item.sms_text }}
              </p>
              <div class="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" :disabled="pending" @click="edit(item)"
                  >Düzenle<span class="sr-only"> {{ item.name }}</span></Button
                ><Button variant="outline" size="sm" :disabled="pending" @click="remove(item)"
                  >Sil<span class="sr-only"> {{ item.name }}</span></Button
                >
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
          /> </CardContent
      ></Card>
      <Card
        ><CardHeader
          ><CardTitle
            ><h2>{{ editingId ? 'Şablonu düzenle' : 'Yeni şablon' }}</h2></CardTitle
          ></CardHeader
        ><CardContent>
          <form class="grid gap-4" @submit.prevent="submit">
            <div class="grid gap-2">
              <Label for="template-name">Şablon adı</Label
              ><Input
                id="template-name"
                v-model="name"
                maxlength="100"
                required
                :readonly="pending"
                placeholder="Örneğin: sepet hatırlatması"
              />
            </div>
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
            <p v-if="feedback" role="status" class="text-sm">{{ feedback }}</p>
            <div class="flex flex-wrap gap-2">
              <Button type="submit" :disabled="pending || !dirty">{{
                pending ? 'Kaydediliyor…' : 'Şablonu kaydet'
              }}</Button
              ><Button type="button" variant="outline" :disabled="pending" @click="edit()"
                >Yeni şablon</Button
              >
            </div>
          </form>
        </CardContent></Card
      >
    </div>
  </main>
</template>
