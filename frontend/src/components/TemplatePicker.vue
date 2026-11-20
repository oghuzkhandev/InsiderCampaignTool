<script setup lang="ts">
import { ref, watch } from 'vue'
import { ArrowDownToLineIcon, LayoutTemplateIcon, MailIcon, MessageSquareIcon } from '@lucide/vue'
import type { ContentFields } from '@/api/campaigns'
import { getTemplates, type ContentTemplate } from '@/api/workspace'
import { useResource } from '@/composables/useResource'
import ErrorPanel from '@/components/ErrorPanel.vue'
import ListPagination from '@/components/ListPagination.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
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
  <!-- `contents`: kök kendi kutusunu oluşturmaz; düğme ve açılan liste, bileşeni barındıran satırın
       doğrudan öğeleri olur (düğme diğer eylemlerle yan yana, liste en altta ve tam genişlikte). -->
  <div class="contents">
    <Button
      type="button"
      variant="outline"
      size="lg"
      class="w-full sm:w-auto"
      :disabled="disabled"
      :aria-expanded="open"
      @click="open = !open"
    >
      <LayoutTemplateIcon aria-hidden="true" />
      {{ open ? 'Şablon listesini kapat' : 'Şablondan doldur' }}
    </Button>
    <section
      v-if="open"
      aria-label="Şablon seç"
      class="order-last grid min-w-0 w-full basis-full gap-5 rounded-xl border p-4 sm:p-5"
    >
      <div class="grid gap-2">
        <h3 class="text-base font-semibold">Kayıtlı şablonlardan seçin</h3>
        <p class="text-muted-foreground text-sm leading-relaxed">
          SMS ve e-posta metinlerini inceleyin, ardından kullanmak istediğiniz şablonu seçin. Seçim
          yalnızca düzenleyiciyi doldurur; gönderim başlatmaz. Kampanyaya kaydetmek için yeni sürüm
          oluşturun.
        </p>
      </div>
      <Separator />
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
      <ul v-else class="grid min-w-0 gap-4">
        <li v-for="item in data.items" :key="item.id" class="min-w-0">
          <Card class="gap-4" data-testid="template-option">
            <CardHeader class="gap-1.5 pb-0">
              <p class="text-muted-foreground text-xs">Şablon adı</p>
              <CardTitle>
                <h4 class="text-base wrap-anywhere">{{ item.name }}</h4>
              </CardTitle>
            </CardHeader>
            <CardContent class="grid min-w-0 gap-4">
              <Separator />
              <dl class="grid min-w-0 gap-4 text-sm">
                <div class="grid gap-2">
                  <dt class="text-muted-foreground flex items-center gap-2 font-medium">
                    <MessageSquareIcon class="size-4 shrink-0" aria-hidden="true" />
                    SMS metni
                  </dt>
                  <dd class="leading-relaxed whitespace-pre-wrap wrap-anywhere">
                    {{ item.sms_text }}
                  </dd>
                </div>
                <div class="grid gap-3 border-t pt-4">
                  <dt class="text-muted-foreground flex items-center gap-2 font-medium">
                    <MailIcon class="size-4 shrink-0" aria-hidden="true" />
                    E-posta
                  </dt>
                  <dd class="grid min-w-0 gap-3">
                    <div class="grid gap-1">
                      <span class="text-muted-foreground text-xs">Konu</span>
                      <p class="font-medium wrap-anywhere">{{ item.email_subject }}</p>
                    </div>
                    <div class="grid gap-1">
                      <span class="text-muted-foreground text-xs">Gövde önizlemesi</span>
                      <p
                        class="text-foreground line-clamp-3 leading-relaxed whitespace-pre-wrap wrap-anywhere"
                      >
                        {{ item.email_body }}
                      </p>
                    </div>
                  </dd>
                </div>
              </dl>
              <Separator />
              <Button
                type="button"
                variant="outline"
                class="w-full sm:w-auto sm:justify-self-end"
                :disabled="disabled"
                @click="select(item)"
              >
                <ArrowDownToLineIcon aria-hidden="true" />
                Bu şablonu kullan<span class="sr-only">: {{ item.name }}</span>
              </Button>
            </CardContent>
          </Card>
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
    </section>
  </div>
</template>
