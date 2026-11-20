<script setup lang="ts">
import {
  CheckIcon,
  FileTextIcon,
  MessageSquareTextIcon,
  PencilLineIcon,
  SaveIcon,
  ShieldCheckIcon,
  SparklesIcon,
} from '@lucide/vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ApiError, isAbortError, type ApiReason } from '@/api/campaigns'
import {
  approveCampaign,
  generateContent,
  getCampaign,
  saveContent,
  type ApproveResult,
  type CampaignDetail,
  type ContentFields,
  type ContentVersion,
} from '@/api/content'
import { applyDelta, emptyFields, hasText, sameFields } from '@/content/draft'
import { FIELD_LABELS, contentErrorTitle, contentReasonMessage } from '@/content/messages'
import { useUnsavedChangesGuard, UNSAVED_MESSAGE } from '@/content/useUnsavedChangesGuard'
import ApprovePanel from '@/components/ApprovePanel.vue'
import AudienceCounts from '@/components/AudienceCounts.vue'
import TemplatePicker from '@/components/TemplatePicker.vue'
import ContentEditor from '@/components/ContentEditor.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import MessagePreview from '@/components/MessagePreview.vue'
import SegmentSummary from '@/components/SegmentSummary.vue'
import CampaignSteps from '@/components/CampaignSteps.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import StepNav, { type NavStep } from '@/components/StepNav.vue'
import { notifyCampaignsChanged } from '@/campaign/recent'
import { stepStatuses } from '@/campaign/steps'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'

const props = defineProps<{ id: string }>()

const loading = ref(true)
const loadError = ref<ApiError | null>(null)
const detail = ref<CampaignDetail | null>(null)

/** Son kaydedilmiş (doğrulanmış) sürüm; onaylanabilen yalnızca odur. */
const saved = ref<ContentVersion | null>(null)
/** Düzenleyicideki metin: kayıtlı sürümden farklıysa kaydedilmemiş değişiklik vardır. */
const draft = ref<ContentFields>(emptyFields())
/** Metin doğrulanamayan bir üretimden geldi (kaydedilmedi, doğrulanmamış). */
const fromPartial = ref(false)
/** Kullanıcı üretmeden alanları elle doldurmak istedi. */
const manualEntry = ref(false)

const generating = ref(false)
const saving = ref(false)
const approving = ref(false)
const generateError = ref<ApiError | null>(null)
const saveError = ref<ApiError | null>(null)
const approveError = ref<ApiError | null>(null)
const approveResult = ref<ApproveResult | null>(null)
/** Son başarılı işlemin kısa geri bildirimi ("Kaydedildi · sürüm 2"); kullanıcı metni değiştirince gizlenir. */
const feedback = ref<string | null>(null)

const controllers = new Set<AbortController>()

function track(): AbortController {
  const controller = new AbortController()
  controllers.add(controller)
  return controller
}

function done(controller: AbortController) {
  controllers.delete(controller)
}

function toApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
}

const fieldsOf = (v: ContentVersion): ContentFields => ({
  sms_text: v.sms_text,
  email_subject: v.email_subject,
  email_body: v.email_body,
})

function applyTemplate(fields: ContentFields) {
  if (busy.value || isApproved.value || (dirty.value && !window.confirm(UNSAVED_MESSAGE))) return
  draft.value = { ...fields }
  manualEntry.value = true
  fromPartial.value = false
  generateError.value = saveError.value = null
  feedback.value = 'Şablon düzenleyiciye aktarıldı. Kaydetmeden onaylanamaz.'
}

function adopt(version: ContentVersion | undefined) {
  saved.value = version ?? null
  draft.value = version ? fieldsOf(version) : emptyFields()
  fromPartial.value = false
}

async function load() {
  controllers.forEach((c) => c.abort())
  controllers.clear()
  // İptal edilen işlemlerin finally blokları artık çalışmaz (controller kümeden
  // çıktı), bu yüzden işlem bayrakları ve önceki kampanyanın taslağı burada
  // sıfırlanır; aksi halde yeni kampanyada düğmeler kilitli kalır.
  // İptal yalnızca istemci tarafıdır ve commit edilmiş bir kaydı (sürüm, onay)
  // geri almaz; hangisinin olduğunu istemci bilemez, bu yüzden kampanyaya dönünce
  // durum sunucudan yeniden okunur.
  generating.value = saving.value = approving.value = false
  adopt(undefined)
  loading.value = true
  loadError.value = null
  detail.value = null
  generateError.value = saveError.value = approveError.value = null
  approveResult.value = null
  feedback.value = null
  manualEntry.value = false

  const controller = track()
  try {
    const result = await getCampaign(props.id, controller.signal)
    if (!controllers.has(controller)) return
    detail.value = result
    adopt(result.content)
  } catch (err) {
    if (!controllers.has(controller) || isAbortError(err)) return
    loadError.value = toApiError(err)
  } finally {
    if (controllers.has(controller)) loading.value = false
    done(controller)
  }
}

/** Başka bir yerde değişen kampanyayı (ör. onaylandı) sessizce yeniden okur. */
async function refresh() {
  const controller = track()
  try {
    const result = await getCampaign(props.id, controller.signal)
    if (!controllers.has(controller)) return
    detail.value = result
    adopt(result.content)
  } catch (err) {
    if (!isAbortError(err)) loadError.value = toApiError(err)
  } finally {
    done(controller)
  }
}

watch(() => props.id, load, { immediate: true })
onBeforeUnmount(() => controllers.forEach((c) => c.abort()))

const isApproved = computed(() => detail.value !== null && detail.value.status !== 'draft')
const busy = computed(() => generating.value || saving.value || approving.value)
const dirty = computed(() => {
  if (saved.value) return !sameFields(draft.value, fieldsOf(saved.value))
  return hasText(draft.value)
})
// Kaydedilmemiş metin varken sayfadan ayrılmak içeriği kaybettirir: kullanıcı uyarılır.
useUnsavedChangesGuard(() => dirty.value && !isApproved.value)
const showEditor = computed(
  () => generating.value || saved.value !== null || fromPartial.value || manualEntry.value,
)
const canApprove = computed(() => saved.value !== null && !dirty.value && !busy.value)
// Henüz içerik yokken sayfanın ana eylemi üretmektir; içerik varken ana eylem kaydetmek
// (ve sonra onaylamaktır), yeniden üretme ikincil kalır.
const generateIsPrimary = computed(
  () => saved.value === null && !fromPartial.value && !manualEntry.value,
)
// Soldaki düğme önceki aşamaya DÖNMEZ: önizleme ekranı var olan bir kampanyayı yüklemez, yeni bir
// brief ile yeni kampanya başlatır (bu kampanya olduğu gibi kayıtlı kalır). Kullanıcı "geri
// döndüğünü" sanmasın diye "Geri" değil kendi adıyla ("Yeni kampanya") anılır. "İleri":
// onaylanınca gönderim takibi; onaydan önce kapalıdır. Kaydedilmemiş metin varken ayrılmak
// her iki durumda da uyarılır.
const backStep: NavStep = {
  label: 'Yeni kampanya',
  text: '+ Yeni kampanya',
  to: '/',
  title: 'Yeni bir kampanya başlatır. Bu kampanya olduğu gibi kayıtlı kalır.',
}
const nextStep = computed<NavStep>(() =>
  isApproved.value
    ? { label: 'Gönderimi izle', to: `/campaigns/${props.id}/delivery`, testid: 'delivery-link' }
    : { label: 'Gönderimi izle', hint: 'Önce içeriği onaylayın.' },
)
// Adım göstergesi: kaydedilmiş ve değişmemiş içerik varsa içerik adımı biter, onay sürer.
const steps = computed(() =>
  stepStatuses({
    status: detail.value?.status ?? null,
    contentReady: saved.value !== null && !dirty.value,
  }),
)

async function generate() {
  if (busy.value) return
  const controller = track()
  generating.value = true
  feedback.value = null
  generateError.value = saveError.value = approveError.value = null
  fromPartial.value = false
  draft.value = emptyFields() // canlı geçici önizleme

  try {
    const version = await generateContent(
      props.id,
      (delta) => {
        draft.value = applyDelta(draft.value, delta)
      },
      controller.signal,
    )
    if (!controllers.has(controller)) return
    adopt(version)
    feedback.value = `Üretildi · sürüm ${version.content_version}`
  } catch (err) {
    if (!controllers.has(controller) || isAbortError(err)) return
    const error = toApiError(err)
    generateError.value = error
    if (error.partial) {
      // Doğrulanamayan ama çözümlenebilen metinler düzenleme için açılır.
      draft.value = { ...error.partial }
      fromPartial.value = true
    } else {
      // Başarısız üretim önceki kayıtlı sürümü bozmaz.
      adopt(saved.value ?? undefined)
    }
    if (error.code === 'CAMPAIGN_NOT_DRAFT') void refresh()
  } finally {
    if (controllers.has(controller)) generating.value = false
    done(controller)
  }
}

async function save() {
  if (busy.value || !dirty.value) return
  const controller = track()
  saving.value = true
  feedback.value = null
  saveError.value = approveError.value = null
  try {
    const version = await saveContent(props.id, draft.value, controller.signal)
    if (!controllers.has(controller)) return
    adopt(version)
    generateError.value = null
    feedback.value = `Kaydedildi · sürüm ${version.content_version}`
  } catch (err) {
    if (!controllers.has(controller) || isAbortError(err)) return
    const error = toApiError(err)
    saveError.value = error
    if (error.code === 'CAMPAIGN_NOT_DRAFT') void refresh()
  } finally {
    if (controllers.has(controller)) saving.value = false
    done(controller)
  }
}

async function approve() {
  const version = saved.value
  if (!version || !canApprove.value) return
  const controller = track()
  approving.value = true
  feedback.value = null
  approveError.value = null
  try {
    const result = await approveCampaign(props.id, version.content_version, controller.signal)
    if (!controllers.has(controller) || !detail.value) return
    approveResult.value = result
    detail.value = {
      ...detail.value,
      status: result.status,
      approved_content_version: result.approved_content_version,
      approved_at: result.approved_at,
    }
    // Kenar çubuğundaki liste kampanyanın yeni durumunu göstersin.
    notifyCampaignsChanged()
  } catch (err) {
    if (!controllers.has(controller) || isAbortError(err)) return
    const error = toApiError(err)
    approveError.value = error
    // Gördüğünüz sürüm eski veya kampanya değişmiş: son durumu yükle.
    if (
      ['STALE_CONTENT_VERSION', 'CAMPAIGN_ALREADY_APPROVED', 'NO_VALIDATED_CONTENT'].includes(
        error.code,
      )
    ) {
      void refresh()
    }
  } finally {
    if (controllers.has(controller)) approving.value = false
    done(controller)
  }
}

/** Doğrulama sebeplerini alana göre (alan altı) ve alansız (panel) ayırır. */
function splitReasons(error: ApiError | null): {
  byField: Record<string, string[]>
  general: string[]
} {
  const byField: Record<string, string[]> = {}
  const general: string[] = []
  if (!error) return { byField, general }
  for (const reason of error.reasons) {
    const message = contentReasonMessage(reason)
    if (reason.field && FIELD_LABELS[reason.field] && error.code === 'CONTENT_INVALID') {
      ;(byField[reason.field] ??= []).push(message)
    } else {
      general.push(message)
    }
  }
  return { byField, general }
}

const generateSplit = computed(() => splitReasons(generateError.value))
const saveSplit = computed(() => splitReasons(saveError.value))
const problems = computed(() => {
  const merged: Record<string, string[]> = {}
  for (const source of [generateSplit.value.byField, saveSplit.value.byField]) {
    for (const [field, list] of Object.entries(source)) (merged[field] ??= []).push(...list)
  }
  return merged
})

const approveReasons = computed(() =>
  (approveError.value?.reasons ?? []).map((r: ApiReason) => contentReasonMessage(r)),
)

// Kullanıcı metni değiştirince eski doğrulama mesajları geçerliliğini yitirir.
watch(draft, () => {
  if (!generating.value && saveError.value?.code === 'CONTENT_INVALID') saveError.value = null
})

const sourceLabel = computed(() =>
  saved.value?.source === 'llm' ? 'Yapay zekâ ile üretildi' : 'Elle düzenlendi',
)
const approvedAtText = computed(() => {
  const at = detail.value?.approved_at
  return at ? new Date(at).toLocaleString('tr-TR') : ''
})
</script>

<template>
  <main class="grid gap-6">
    <PageHeader
      title="Kampanya"
      description="İçeriği üretin veya düzenleyin, kaydettiğiniz sürümü onaylayın."
    >
      <template #badge>
        <StatusBadge v-if="detail" :status="detail.status" data-testid="campaign-state" />
      </template>
      <CampaignSteps v-if="detail" :statuses="steps" />
    </PageHeader>

    <div v-if="loading" class="grid gap-3" role="status" data-testid="loading">
      <span class="sr-only">Kampanya yükleniyor…</span>
      <Skeleton class="h-5 w-2/3" />
      <Skeleton class="h-40 w-full" />
    </div>
    <ErrorPanel
      v-else-if="loadError"
      :title="contentErrorTitle(loadError)"
      :reasons="[]"
      :retryable="loadError.retryable"
      @retry="load"
    />

    <!-- Geniş ekranda iki sütun: solda brief ve geniş düzenleyici, sağda kompakt kitle ve onay
         özeti. Dar ekranda alt alta: brief, içerik, kitle, onay. -->
    <div v-else-if="detail" class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div class="grid gap-6">
        <Card role="region" aria-labelledby="brief-title">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <FileTextIcon class="text-primary size-5" aria-hidden="true" />
              <h2 id="brief-title">Brief</h2>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p class="text-sm wrap-anywhere whitespace-pre-wrap">{{ detail.brief }}</p>
          </CardContent>
        </Card>

        <Card role="region" aria-labelledby="content-title">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <MessageSquareTextIcon class="text-primary size-5" aria-hidden="true" />
              <h2 id="content-title">İçerik</h2>
            </CardTitle>
            <CardDescription v-if="saved" data-testid="version">
              Sürüm {{ saved.content_version }} · {{ sourceLabel }}
            </CardDescription>
          </CardHeader>

          <CardContent class="grid gap-4">
            <TemplatePicker v-if="!isApproved" :disabled="busy" @select="applyTemplate" />
            <div v-if="!isApproved" class="flex flex-col gap-2 sm:flex-row">
              <Button
                type="button"
                size="lg"
                :variant="generateIsPrimary ? 'default' : 'outline'"
                class="w-full sm:w-auto"
                :disabled="busy"
                @click="generate"
              >
                <SparklesIcon aria-hidden="true" />
                {{
                  generating ? 'Üretiliyor…' : saved || fromPartial ? 'Yeniden üret' : 'İçerik üret'
                }}
              </Button>
              <Button
                v-if="!showEditor"
                type="button"
                variant="outline"
                size="lg"
                class="w-full sm:w-auto"
                :disabled="busy"
                @click="manualEntry = true"
              >
                <PencilLineIcon aria-hidden="true" />
                İçeriği elle yaz
              </Button>
            </div>

            <div
              v-if="generating"
              class="text-muted-foreground flex items-start gap-2 text-sm"
              role="status"
            >
              <!-- Spinner svg'si kendi `status` rolünü taşır; metin zaten bu bölgede okunur. -->
              <Spinner
                class="mt-0.5 shrink-0 motion-reduce:animate-none"
                role="presentation"
                aria-hidden="true"
              />
              <span>
                İçerik üretiliyor… Gördüğünüz metin geçicidir; doğrulanınca sürüm olarak kaydedilir.
              </span>
            </div>

            <ErrorPanel
              v-if="generateError"
              :title="contentErrorTitle(generateError)"
              :reasons="generateSplit.general"
              :retryable="generateError.code !== 'CAMPAIGN_NOT_DRAFT'"
              retry-label="Yeniden üret"
              hint=""
              @retry="generate"
            />
            <Alert v-if="fromPartial" role="status" data-testid="partial-notice">
              <AlertDescription>
                Üretilen metin doğrulanamadı ve henüz kaydedilmedi. Aşağıdaki metni düzeltip
                kaydedin veya yeniden üretin; onay yalnızca kaydedilmiş sürümle yapılabilir.
              </AlertDescription>
            </Alert>

            <ContentEditor
              v-if="showEditor"
              v-model="draft"
              :readonly="generating || saving || isApproved"
              :problems="problems"
            />

            <!-- Canlı önizleme: yazarken mesajın alıcıya nasıl görüneceğinin örneği. -->
            <section
              v-if="showEditor && hasText(draft)"
              class="grid gap-2 rounded-lg border p-3"
              aria-labelledby="preview-title"
              data-testid="editor-preview"
            >
              <h3 id="preview-title" class="text-sm font-medium">Mesaj önizlemesi</h3>
              <MessagePreview :fields="draft" />
            </section>

            <div
              v-if="showEditor && !isApproved"
              class="flex flex-col gap-2 sm:flex-row sm:items-center"
            >
              <Button
                type="button"
                size="lg"
                class="w-full sm:w-auto"
                :disabled="busy || !dirty"
                @click="save"
              >
                <SaveIcon aria-hidden="true" />
                {{ saving ? 'Kaydediliyor…' : 'Yeni sürüm olarak kaydet' }}
              </Button>
              <span v-if="dirty && !generating" class="text-muted-foreground text-sm">
                Kaydedilmemiş değişiklik var.
              </span>
              <!-- Her zaman var olan durum bölgesi: metin değişince ekran okuyucuya bildirilir. -->
              <span
                role="status"
                class="inline-flex items-center gap-1 text-sm font-medium empty:hidden"
                data-testid="action-feedback"
              >
                <template v-if="feedback && !dirty">
                  <CheckIcon class="size-4" aria-hidden="true" />
                  {{ feedback }}
                </template>
              </span>
            </div>
            <ErrorPanel
              v-if="saveError"
              :title="contentErrorTitle(saveError)"
              :reasons="saveSplit.general"
              :retryable="false"
              hint=""
            />
          </CardContent>
        </Card>
      </div>

      <!-- Dar ekranda `contents`: kartlar üst gride katılır ve `order` ile dizilir; onay özeti
           içerikten hemen sonra gelir (ana eyleme kaydırmadan ulaşılır), kitle ve koşullar
           onun ardından. Geniş ekranda sağ sütunda yapışkan bir bölümdür. -->
      <aside class="grid gap-6 max-lg:contents lg:sticky lg:top-6" aria-label="Kitle ve onay özeti">
        <AudienceCounts class="max-lg:order-2" compact :audience="detail.audience" />
        <SegmentSummary class="max-lg:order-2" :segment="detail.segment" />

        <ApprovePanel
          v-if="saved && !isApproved"
          class="max-lg:order-1"
          :version="saved.content_version"
          :content="fieldsOf(saved)"
          :audience="detail.audience"
          :disabled="!canApprove"
          :pending="approving"
          @approve="approve"
        />
        <ErrorPanel
          v-if="approveError"
          class="max-lg:order-1"
          :title="contentErrorTitle(approveError)"
          :reasons="approveReasons"
          :retryable="false"
          hint=""
        />

        <Card v-if="isApproved" class="max-lg:order-1" role="status" data-testid="approved">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <ShieldCheckIcon
                class="size-5 shrink-0 text-emerald-700 dark:text-emerald-300"
                aria-hidden="true"
              />
              Kampanya onaylandı: sürüm {{ detail.approved_content_version }}
              <template v-if="approvedAtText"> · {{ approvedAtText }}</template>
            </CardTitle>
            <CardDescription v-if="approveResult" data-testid="queued">
              {{ approveResult.queued.sms }} SMS ve {{ approveResult.queued.email }} e-posta mesajı
              gönderim için kuyruğa alındı.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-muted-foreground text-sm">
              İçerik artık değiştirilemez. Gönderimin durumunu "İleri" ile izleyebilirsiniz.
            </p>
          </CardContent>
        </Card>
      </aside>
    </div>

    <StepNav sticky :back="backStep" :next="nextStep" />
  </main>
</template>
