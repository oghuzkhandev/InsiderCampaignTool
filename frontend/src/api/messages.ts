import { ApiError } from './campaigns'

/** Hata için kullanıcıya gösterilen başlık (Türkçe). */
export function errorTitle(error: ApiError): string {
  switch (error.code) {
    case 'SEGMENT_INVALID':
      return "Brief'ten geçerli bir kitle tanımı çıkarılamadı."
    case 'BRIEF_EMPTY':
      return 'Brief boş olamaz.'
    case 'BRIEF_TOO_LONG':
      return 'Brief çok uzun.'
    case 'LLM_TIMEOUT':
      return 'Kitle tanımı üretimi zaman aşımına uğradı.'
    case 'LLM_RATE_LIMITED':
      return 'Kitle tanımı servisi şu an yoğun.'
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
