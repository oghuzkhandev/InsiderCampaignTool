// İçerik üretimi, düzenleme ve onay hatalarının Türkçe metinleri. Model veya
// kullanıcı metni ham olarak gösterilmez; yalnızca sebep kodu ve kısa alan/ad
// bilgisi kullanılır. Değerler Vue tarafından metin olarak basılır.
import type { ApiError, ApiReason } from '@/api/campaigns'
import { MAX_SMS_LENGTH, MAX_SUBJECT_LENGTH, NAME_VARIABLE } from './draft'

export const FIELD_LABELS: Record<string, string> = {
  sms_text: 'SMS metni',
  email_subject: 'E-posta konusu',
  email_body: 'E-posta gövdesi',
}

function fieldLabel(field: string | undefined): string {
  return (field && FIELD_LABELS[field]) || 'Bir alan'
}

function limitOf(field: string | undefined): number | undefined {
  if (field === 'sms_text') return MAX_SMS_LENGTH
  if (field === 'email_subject') return MAX_SUBJECT_LENGTH
  return undefined
}

/** Hata için kullanıcıya gösterilen başlık. */
export function contentErrorTitle(error: ApiError): string {
  switch (error.code) {
    case 'SCHEDULE_IN_PAST':
      return 'Seçilen gönderim zamanı geçti. Gelecekte bir saat seçip yeniden onaylayın.'
    case 'SCHEDULE_CONFLICT':
      return 'Kampanya farklı bir gönderim zamanı ile zaten onaylandı. Güncel planı kontrol edin.'
    case 'CONTENT_INVALID':
      return 'İçerik doğrulanamadı.'
    case 'LLM_TIMEOUT':
      return 'İçerik üretimi zaman aşımına uğradı.'
    case 'LLM_RATE_LIMITED':
      return 'İçerik servisi şu an yoğun.'
    case 'STREAM_INTERRUPTED':
      return 'İçerik üretimi yarıda kesildi.'
    case 'CAMPAIGN_NOT_DRAFT':
      return 'Kampanya onaylandı; içerik artık değiştirilemez.'
    case 'CAMPAIGN_NOT_FOUND':
      return 'Kampanya bulunamadı.'
    case 'STALE_CONTENT_VERSION':
      return 'Gördüğünüz içerik sürümü güncel değil. Son sürüm yüklendi; kontrol edip yeniden onaylayın.'
    case 'NO_VALIDATED_CONTENT':
      return 'Onaylanabilir (doğrulanmış) bir içerik sürümü yok.'
    case 'CAMPAIGN_ALREADY_APPROVED':
      return 'Kampanya başka bir içerik sürümüyle zaten onaylandı.'
    case 'RENDER_LIMIT_EXCEEDED':
      return 'Kişiselleştirme sonrası bazı alıcıların metni sınırı aşıyor. Metni kısaltın.'
    case 'NO_ELIGIBLE_RECIPIENTS':
      return 'Hiçbir kanalda uygun alıcı yok; kampanya onaylanamaz.'
    case 'UNFILTERED_SEGMENT':
      return 'Filtresiz kitle gönderime onaylanamaz.'
    case 'REQUEST_TOO_LARGE':
      return 'İçerik çok büyük.'
    case 'INVALID_REQUEST':
      return 'İstek geçerli değil.'
    case 'NETWORK':
      return 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.'
    case 'UNEXPECTED_RESPONSE':
      return 'Sunucudan beklenmeyen bir yanıt alındı.'
    case 'INTERNAL':
      return 'Beklenmeyen bir hata oluştu.'
    default:
      return error.message || 'İstek tamamlanamadı.'
  }
}

/** Doğrulama ve onay sebep kodunu Türkçe açıklamaya çevirir. */
export function contentReasonMessage(reason: ApiReason): string {
  const label = fieldLabel(reason.field)
  switch (reason.code) {
    case 'TOO_LONG': {
      const limit = limitOf(reason.field)
      const max = limit === undefined ? '' : ` ${limit} karakteri`
      if (reason.count !== undefined) {
        // Onayda: kişiselleştirme sonrası, alıcı bazında.
        const example = reason.text ? ` (örnek ad: "${reason.text}")` : ''
        return `${label}, ${reason.count} alıcı için kişiselleştirme sonrası${max} aşıyor${example}.`
      }
      return `${label}${max} aşıyor (${NAME_VARIABLE} yerine yazılacak ad dahil).`
    }
    case 'UNKNOWN_VARIABLE':
      return `${label}: bilinmeyen değişken "{{${reason.text ?? ''}}}". Yalnızca ${NAME_VARIABLE} kullanılabilir.`
    case 'MALFORMED_VARIABLE':
      return `${label}: eşleşmeyen "{{" veya "}}". Değişken ${NAME_VARIABLE} biçiminde olmalı.`
    case 'EMPTY_FIELD':
      return `${label} boş olamaz.`
    case 'MISSING_FIELD':
      return `${label} eksik.`
    case 'INVALID_CHARACTER':
      return reason.field === 'email_subject'
        ? 'E-posta konusu satır sonu içeremez.'
        : `${label} geçersiz bir karakter içeriyor.`
    case 'MALFORMED_OUTPUT':
      return 'Üretilen içerik beklenen biçimde değil.'
    default:
      return 'Bilinmeyen bir sebep nedeniyle reddedildi.'
  }
}
