// Kampanya listesinden hangi ekrana gidileceği (CMP-001): taslak kampanya içerik ve onay
// ekranına, onaylanmış (approved, sending, completed) kampanya gönderim takibine açılır.
// Bilinmeyen bir durum (ileride eklenirse) güvenli taraf olarak kampanya ekranına gider.
import type { RouteLocationRaw } from 'vue-router'

const DELIVERY_STATUSES = new Set(['approved', 'sending', 'completed'])

/** Bu durumdaki kampanyanın gönderimi başlamış mı (gönderim takibi ekranı anlamlı mı)? */
export function opensDelivery(status: string): boolean {
  return DELIVERY_STATUSES.has(status)
}

export function campaignTarget(id: string, status: string): RouteLocationRaw {
  return opensDelivery(status) ? `/campaigns/${id}/delivery` : `/campaigns/${id}`
}

/** Satırın nereye götürdüğünü anlatan kısa metin (bağlantının sonunda görünür). */
export function campaignTargetLabel(status: string): string {
  return opensDelivery(status) ? 'Gönderimi izle' : 'Kampanyayı aç'
}
