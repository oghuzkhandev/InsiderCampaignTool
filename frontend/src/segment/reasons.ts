import type { ApiReason } from '@/api/campaigns'

const FIELD_LABELS: Record<string, string> = {
  city: 'Şehir',
  last_cart_add_at: 'Sepete ekleme tarihi',
  last_purchase_at: 'Satın alma tarihi',
  order_count: 'Sipariş sayısı',
}

const SUPPORTED_FIELDS = 'şehir, sepete ekleme tarihi, satın alma tarihi ve sipariş sayısı'

function fieldLabel(field: string | undefined): string {
  if (!field) return 'Bir alan'
  return FIELD_LABELS[field] ?? `"${field}"`
}

/**
 * Backend'in sebep kodunu Türkçe açıklamaya çevirir. Model çıktısı ham olarak
 * gösterilmez; yalnızca kod ve kısa alan/metin bilgisi kullanılır. Değerler
 * metin olarak basılır (Vue arayüzde HTML olarak yorumlamaz).
 */
export function reasonMessage(reason: ApiReason): string {
  switch (reason.code) {
    case 'UNSUPPORTED_FIELD':
      return `${fieldLabel(reason.field)} alanı desteklenmiyor. Desteklenen alanlar: ${SUPPORTED_FIELDS}.`
    case 'UNSUPPORTED_OPERATOR':
      return `${fieldLabel(reason.field)} için "${reason.text ?? ''}" koşulu desteklenmiyor.`
    case 'UNSUPPORTED_STRUCTURE':
      return "Koşullar yalnızca 've' ile birleştirilebilir. 'Veya' ile gruplama, iç içe koşullar ve çok fazla koşul desteklenmiyor."
    case 'UNSUPPORTED_PART':
      return `Brief'in şu kısmı ifade edilemedi: "${reason.text ?? ''}".`
    case 'EMPTY_FILTER':
      return `Brief'ten hiçbir koşul çıkarılamadı. Tüm kullanıcılara gönderim desteklenmez; ${SUPPORTED_FIELDS} gibi en az bir koşul belirtin.`
    case 'INVALID_RANGE':
      return `${fieldLabel(reason.field)} için değer izin verilen aralıkta değil.`
    case 'INVALID_VALUE_TYPE':
      return `${fieldLabel(reason.field)} için değer geçerli bir türde değil.`
    case 'MALFORMED_OUTPUT':
      return "Brief anlaşılamadı. Brief'i daha açık yazıp yeniden deneyin."
    default:
      return 'Bilinmeyen bir sebep nedeniyle reddedildi.'
  }
}
