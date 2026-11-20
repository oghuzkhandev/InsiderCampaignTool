<script setup lang="ts">
import { ref, watch } from 'vue'
import type { ContentFields } from '@/api/campaigns'
import { getTemplates, type ContentTemplate } from '@/api/workspace'
import { useResource } from '@/composables/useResource'
import ErrorPanel from '@/components/ErrorPanel.vue'
import ListPagination from '@/components/ListPagination.vue'
import { Button } from '@/components/ui/button'
const props = defineProps<{ disabled: boolean }>()
const emit = defineEmits<{ select: [fields: ContentFields] }>()
const open = ref(false)
const page = ref(1)
const { data, loading, error, reload } = useResource((signal) => getTemplates(page.value, signal))
watch([open, page], () => {
  if (open.value) void reload()
})
function select(item: ContentTemplate) {
  if (props.disabled) return
  emit('select', {
    sms_text: item.sms_text,
    email_subject: item.email_subject,
    email_body: item.email_body,
  })
  open.value = false
}
</script>
<template>
  <div class="grid gap-3">
    <Button
      type="button"
      variant="outline"
      class="w-full sm:w-auto sm:justify-self-start"
      :disabled="disabled"
      :aria-expanded="open"
      @click="open = !open"
      >{{ open ? 'Şablon listesini kapat' : 'Şablondan doldur' }}</Button
    >
    <section v-if="open" aria-label="Şablon seç" class="grid gap-3 rounded-lg border p-3">
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
        Henüz şablon yok. İçerik şablonları sayfasından oluşturabilirsiniz.
      </p>
      <ul v-else class="grid gap-2">
        <li v-for="item in data.items" :key="item.id">
          <Button
            variant="outline"
            class="h-auto w-full justify-start text-left whitespace-normal wrap-anywhere"
            :disabled="disabled"
            @click="select(item)"
            >{{ item.name }}</Button
          >
        </li>
      </ul>
      <ListPagination
        v-if="data"
        :page="page"
        :total="data.total"
        :limit="data.limit"
        :loading="loading"
        @change="page = $event"
      />
      <p class="text-muted-foreground text-xs">
        Seçim yalnızca düzenleyiciyi doldurur. Kampanyaya kaydetmek için yeni sürüm oluşturun.
      </p>
    </section>
  </div>
</template>
