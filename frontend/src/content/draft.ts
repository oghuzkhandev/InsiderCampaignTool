// İçerik taslağı için saf yardımcılar: sınırlar, uzunluk ve akış parçalarının
// uygulanması. Sınırlar backend'deki internal/content ile aynıdır (CNT-001).
import type { ContentField, ContentFields, Delta } from '@/api/content'

export const MAX_SMS_LENGTH = 160
export const MAX_SUBJECT_LENGTH = 80
/** Adı olmayan alıcıya yazılan metin (ADR-022). */
export const FALLBACK_NAME = 'Değerli müşterimiz'
export const NAME_VARIABLE = '{{first_name}}'
/** Önizlemede {{first_name}} yerine yazılan örnek ad. Gerçek bir alıcının adı değildir. */
export const SAMPLE_NAME = 'Ayşe'

/**
 * Metindeki {{first_name}} yerine örnek ad koyar (önizleme için). Backend `content.Render`
 * ile aynı biçimde çalışır: yalnızca bu değişken değiştirilir, başka süslü parantez
 * ifadelerine dokunulmaz.
 */
export function renderPreview(text: string, name: string = SAMPLE_NAME): string {
  return text.split(NAME_VARIABLE).join(name)
}

export function emptyFields(): ContentFields {
  return { sms_text: '', email_subject: '', email_body: '' }
}

/** Backend Unicode kod noktası sayar (UTF-16 uzunluğu değil). */
export function runeLength(text: string): number {
  return Array.from(text).length
}

/**
 * Kesin uzunluk alıcı adına bağlıdır ve sunucuda doğrulanır. Ekranda
 * {{first_name}} yerine fallback metninin uzunluğu varsayılarak bir tahmin
 * gösterilir.
 */
export function estimatedLength(text: string): number {
  return runeLength(text.split(NAME_VARIABLE).join(FALLBACK_NAME))
}

/** Akış parçasını önizlemeye uygular; replace önceki metni siler. */
export function applyDelta(fields: ContentFields, delta: Delta): ContentFields {
  const key: ContentField = delta.field
  return { ...fields, [key]: delta.replace ? delta.text : fields[key] + delta.text }
}

export function sameFields(a: ContentFields, b: ContentFields): boolean {
  return (
    a.sms_text === b.sms_text &&
    a.email_subject === b.email_subject &&
    a.email_body === b.email_body
  )
}

export function hasText(fields: ContentFields): boolean {
  return (
    fields.sms_text.trim() !== '' ||
    fields.email_subject.trim() !== '' ||
    fields.email_body.trim() !== ''
  )
}
