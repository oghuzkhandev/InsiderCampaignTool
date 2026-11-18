<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
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
import ApprovePanel from '@/components/ApprovePanel.vue'
import AudienceCounts from '@/components/AudienceCounts.vue'
import ContentEditor from '@/components/ContentEditor.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import SegmentSummary from '@/components/SegmentSummary.vue'

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
const showEditor = computed(
  () => generating.value || saved.value !== null || fromPartial.value || manualEntry.value,
)
const canApprove = computed(() => saved.value !== null && !dirty.value && !busy.value)

async function generate() {
  if (busy.value) return
  const controller = track()
  generating.value = true
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
  saveError.value = approveError.value = null
  try {
    const version = await saveContent(props.id, draft.value, controller.signal)
    if (!controllers.has(controller)) return
    adopt(version)
    generateError.value = null
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
  <main>
    <RouterLink class="back" to="/">← Yeni kampanya</RouterLink>
    <h1>Kampanya</h1>

    <p v-if="loading" class="status" role="status">Kampanya yükleniyor…</p>
    <ErrorPanel
      v-else-if="loadError"
      :title="contentErrorTitle(loadError)"
      :reasons="[]"
      :retryable="loadError.retryable"
      @retry="load"
    />

    <template v-else-if="detail">
      <section class="summary" aria-labelledby="brief-title">
        <h2 id="brief-title">Brief</h2>
        <p class="brief">{{ detail.brief }}</p>
        <p class="state" :class="{ approved: isApproved }">
          {{ isApproved ? 'Onaylandı' : 'Taslak' }}
        </p>
        <AudienceCounts :audience="detail.audience" />
        <SegmentSummary :segment="detail.segment" />
      </section>

      <section class="content" aria-labelledby="content-title">
        <h2 id="content-title">İçerik</h2>
        <p v-if="saved" class="version" data-testid="version">
          Sürüm {{ saved.content_version }} · {{ sourceLabel }}
        </p>

        <div v-if="!isApproved" class="toolbar">
          <button type="button" class="primary" :disabled="busy" @click="generate">
            {{ generating ? 'Üretiliyor…' : saved || fromPartial ? 'Yeniden üret' : 'İçerik üret' }}
          </button>
          <button
            v-if="!showEditor"
            type="button"
            class="secondary"
            :disabled="busy"
            @click="manualEntry = true"
          >
            İçeriği elle yaz
          </button>
        </div>
        <p v-if="generating" class="status" role="status">
          İçerik üretiliyor… Gördüğünüz metin geçicidir; doğrulanınca sürüm olarak kaydedilir.
        </p>

        <ErrorPanel
          v-if="generateError"
          :title="contentErrorTitle(generateError)"
          :reasons="generateSplit.general"
          :retryable="generateError.code !== 'CAMPAIGN_NOT_DRAFT'"
          retry-label="Yeniden üret"
          hint=""
          @retry="generate"
        />
        <p v-if="fromPartial" class="notice" role="status">
          Üretilen metin doğrulanamadı ve henüz kaydedilmedi. Aşağıdaki metni düzeltip kaydedin veya
          yeniden üretin; onay yalnızca kaydedilmiş sürümle yapılabilir.
        </p>

        <ContentEditor
          v-if="showEditor"
          v-model="draft"
          :readonly="generating || saving || isApproved"
          :problems="problems"
        />

        <div v-if="showEditor && !isApproved" class="toolbar">
          <button type="button" class="primary" :disabled="busy || !dirty" @click="save">
            {{ saving ? 'Kaydediliyor…' : 'Yeni sürüm olarak kaydet' }}
          </button>
          <span v-if="dirty && !generating" class="dirty">Kaydedilmemiş değişiklik var.</span>
        </div>
        <ErrorPanel
          v-if="saveError"
          :title="contentErrorTitle(saveError)"
          :reasons="saveSplit.general"
          :retryable="false"
          hint=""
        />
      </section>

      <ApprovePanel
        v-if="saved && !isApproved"
        :version="saved.content_version"
        :audience="detail.audience"
        :disabled="!canApprove"
        :pending="approving"
        @approve="approve"
      />
      <ErrorPanel
        v-if="approveError"
        :title="contentErrorTitle(approveError)"
        :reasons="approveReasons"
        :retryable="false"
        hint=""
      />

      <section v-if="isApproved" class="approved-box" role="status" data-testid="approved">
        <p class="title">
          Kampanya onaylandı: sürüm {{ detail.approved_content_version }}
          <template v-if="approvedAtText"> · {{ approvedAtText }}</template>
        </p>
        <p v-if="approveResult" data-testid="queued">
          {{ approveResult.queued.sms }} SMS ve {{ approveResult.queued.email }} e-posta mesajı
          gönderim için kuyruğa alındı.
        </p>
        <p class="hint">İçerik artık değiştirilemez.</p>
      </section>
    </template>
  </main>
</template>

<style scoped>
main {
  display: grid;
  gap: 1.5rem;
}
h1 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 700;
}
h2 {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 700;
}
.back {
  justify-self: start;
  color: var(--app-accent);
}
.summary,
.content {
  display: grid;
  gap: 0.9rem;
}
.brief {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.state {
  justify-self: start;
  margin: 0;
  padding: 0.15rem 0.6rem;
  font-size: 0.85rem;
  font-weight: 600;
  background: var(--app-warning-bg);
  border: 1px solid var(--app-border);
  border-radius: 999px;
}
.state.approved {
  background: var(--app-surface);
}
.version,
.status,
.hint {
  margin: 0;
  color: var(--app-muted);
  font-size: 0.9rem;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.6rem;
}
.notice {
  margin: 0;
  padding: 0.7rem 0.9rem;
  background: var(--app-warning-bg);
  border: 1px solid var(--app-border);
  border-radius: 8px;
}
.dirty {
  color: var(--app-muted);
  font-size: 0.9rem;
}
.primary,
.secondary {
  padding: 0.6rem 1.2rem;
  font: inherit;
  font-weight: 600;
  border-radius: 8px;
  cursor: pointer;
}
.primary {
  color: var(--app-on-accent);
  background: var(--app-accent);
  border: none;
}
.secondary {
  color: inherit;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
}
button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.approved-box {
  display: grid;
  gap: 0.4rem;
  padding: 1rem;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
}
.approved-box p {
  margin: 0;
}
.approved-box .title {
  font-weight: 700;
}
</style>
