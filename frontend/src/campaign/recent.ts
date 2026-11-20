import { ref } from 'vue'

// Kenar çubuğundaki "Son kampanyalar" listesi (CMP-001) uygulamanın her ekranında görünür.
// Bir ekran kampanya oluşturur veya durumunu değiştirirse (onay gibi) listeyi buradan haberdar
// eder; liste sayacı izleyip kendini yeniler. Ekranlar birbirini ve listeyi tanımaz.
export const campaignListVersion = ref(0)

/** Kampanya oluştu veya durumu değişti: liste yeniden okunsun. */
export function notifyCampaignsChanged(): void {
  campaignListVersion.value += 1
}
