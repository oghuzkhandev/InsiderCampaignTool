import { inject, onBeforeUnmount, onMounted } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, routerKey } from 'vue-router'

/** Kullanıcıya gösterilen uyarı: içerik kaydedilmeden ayrılırsa kaybolur. */
export const UNSAVED_MESSAGE =
  'Kaydedilmemiş değişiklikleriniz var. Sayfadan ayrılırsanız bu değişiklikler kaybolur. Ayrılmak istiyor musunuz?'

/**
 * Kaydedilmemiş değişiklik varken kullanıcı sayfadan ayrılırsa uyarır:
 *  - sekmeyi kapatma, yenileme ve adres değiştirme: tarayıcının kendi uyarısı (`beforeunload`;
 *    metni tarayıcı belirler, sayfa özel metin gösteremez);
 *  - uygulama içindeki gezinme (geri/ileri bağlantıları, adres geçmişi, başka kampanyaya geçiş):
 *    onay penceresi. Kullanıcı vazgeçerse gezinme iptal edilir, sayfa ve metin olduğu gibi kalır.
 * Değişiklik yoksa hiçbir şey sorulmaz.
 */
export function useUnsavedChangesGuard(
  isDirty: () => boolean,
  confirmLeave: (message: string) => boolean = (message) => window.confirm(message),
) {
  function onBeforeUnload(event: BeforeUnloadEvent) {
    if (isDirty()) event.preventDefault()
  }
  onMounted(() => window.addEventListener('beforeunload', onBeforeUnload))
  onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))

  // Yönlendirici olmayan ortamda (bileşen testleri) yalnızca tarayıcı uyarısı kurulur.
  // `inject` varsayılanla çağrılır: `useRouter()` yönlendirici yoksa uyarı basardı.
  if (inject(routerKey, null)) {
    const allowLeave = () => (isDirty() ? confirmLeave(UNSAVED_MESSAGE) : true)
    onBeforeRouteLeave(allowLeave)
    // Aynı bileşen yeniden kullanılırken (/campaigns/A → /campaigns/B) çıkış olayı gelmez.
    onBeforeRouteUpdate(allowLeave)
  }
}
