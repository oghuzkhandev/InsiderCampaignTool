// Bir kampanyanın canlı gönderim durumunu izleyen bağlantının yaşam döngüsü:
// bağlanma, kopunca yeniden bağlanma, takılan bağlantıyı fark etme ve temizlik.
// Ağ ve yanıt doğrulaması api/delivery.ts'dedir; burada yalnızca bağlantının
// durumu yönetilir.
import { onBeforeUnmount, ref, watch } from 'vue'
import { ApiError, isAbortError } from '@/api/campaigns'
import { streamDeliverySnapshots, type DeliverySnapshot } from '@/api/delivery'

/**
 * - idle: izlenecek kampanya yok.
 * - connecting: ilk bağlantı kuruluyor, henüz durum görüntüsü gelmedi.
 * - live: bağlı; son görüntü güncel.
 * - reconnecting: bağlantı koptu; görüntü son bilinen durumdur.
 * - completed: kampanya tamamlandı, canlı takip bilerek kapatıldı.
 * - failed: yeniden bağlanma pes etti veya hata yeniden denemeyle düzelmez.
 */
export type DeliveryConnection =
  'idle' | 'connecting' | 'live' | 'reconnecting' | 'completed' | 'failed'

export interface DeliveryStreamOptions {
  /** n. ardışık başarısızlıktan sonra yeniden bağlanmadan önce beklenecek süre. */
  retryDelayMs?: (failures: number) => number
  /** Bu kadar ardışık başarısızlıktan sonra vazgeçilir (failed). */
  maxFailures?: number
  /**
   * Bu süre boyunca durum görüntüsü gelmezse bağlantı takılmış sayılır. Sunucu her
   * saniye görüntü yollar (TRK-001); takılan bir bağlantı (ör. araya giren proxy
   * akışı tamponlarsa) eski sayıları "canlı" gibi göstermemelidir.
   */
  stallTimeoutMs?: number
}

const defaultRetryDelayMs = (failures: number) => Math.min(1000 * 2 ** (failures - 1), 10_000)

/**
 * `campaignId()` bir kimlik döndürdüğünde o kampanyayı izler; null döndürdüğünde
 * bağlantıyı kapatır. Kimlik değişince önceki bağlantı iptal edilir ve eski
 * kampanyadan sonradan gelen olaylar hiçbir şeyi değiştirmez. Bileşen kaldırılınca
 * bağlantı kapanır.
 */
export function useDeliveryStream(
  campaignId: () => string | null,
  options: DeliveryStreamOptions = {},
) {
  const retryDelayMs = options.retryDelayMs ?? defaultRetryDelayMs
  const maxFailures = options.maxFailures ?? 5
  const stallTimeoutMs = options.stallTimeoutMs ?? 8000

  const snapshot = ref<DeliverySnapshot | null>(null)
  const connection = ref<DeliveryConnection>('idle')
  /** Ardışık başarısız bağlantı denemesi; bir görüntü gelince sıfırlanır. */
  const failures = ref(0)
  /** Son hata (failed durumunda). */
  const error = ref<ApiError | null>(null)
  /** Son görüntünün alındığı an. */
  const updatedAt = ref<Date | null>(null)

  // Her izleme oturumu bir "nesil" numarası alır; eski oturumdan geç gelen bir
  // olay veya sonuç, nesil artık güncel olmadığı için yok sayılır.
  let generation = 0
  let session: AbortController | null = null

  function stop() {
    generation++
    session?.abort()
    session = null
  }

  function start(id: string | null) {
    stop()
    snapshot.value = null
    updatedAt.value = null
    error.value = null
    failures.value = 0
    if (id === null) {
      connection.value = 'idle'
      return
    }
    connection.value = 'connecting'
    session = new AbortController()
    void run(id, generation, session.signal)
  }

  async function run(id: string, mine: number, signal: AbortSignal) {
    while (mine === generation) {
      const outcome = await connectOnce(id, mine, signal)
      if (mine !== generation) return
      if (outcome === 'completed') {
        connection.value = 'completed'
        return
      }
      if (outcome === 'stopped') return

      failures.value += 1
      if (!outcome.retryable || failures.value >= maxFailures) {
        error.value = outcome
        connection.value = 'failed'
        return
      }
      connection.value = 'reconnecting'
      if (!(await sleep(retryDelayMs(failures.value), signal))) return
    }
  }

  /**
   * Tek bir bağlantı denemesi. 'completed': kampanya tamamlandı ve bağlantı bilerek
   * kapatıldı. 'stopped': dışarıdan iptal edildi. Aksi halde bağlantıyı sonlandıran
   * hata döner.
   */
  async function connectOnce(
    id: string,
    mine: number,
    signal: AbortSignal,
  ): Promise<'completed' | 'stopped' | ApiError> {
    const attempt = new AbortController()
    const abortAttempt = () => attempt.abort()
    signal.addEventListener('abort', abortAttempt, { once: true })

    let finished = false
    let stalled = false
    let watchdog: ReturnType<typeof setTimeout> | undefined
    const armWatchdog = () => {
      clearTimeout(watchdog)
      watchdog = setTimeout(() => {
        stalled = true
        attempt.abort()
      }, stallTimeoutMs)
    }

    try {
      armWatchdog()
      await streamDeliverySnapshots(
        id,
        (received) => {
          if (mine !== generation) return
          snapshot.value = received
          updatedAt.value = new Date()
          failures.value = 0
          if (received.status === 'completed') {
            // Tamamlanan kampanyada artık değişen bir şey yok; bağlantı kapatılır.
            // Durum hemen yansır: bağlantının gerçekten kapanması beklenmez.
            finished = true
            connection.value = 'completed'
            clearTimeout(watchdog)
            attempt.abort()
            return
          }
          connection.value = 'live'
          armWatchdog()
        },
        attempt.signal,
      )
      // streamDeliverySnapshots normal dönmez; dönerse bağlantı kesilmiş sayılır.
      return new ApiError(0, 'STREAM_INTERRUPTED', 'Gönderim takibi bağlantısı koptu.')
    } catch (err) {
      if (finished) return 'completed'
      if (stalled) {
        return new ApiError(0, 'STREAM_INTERRUPTED', 'Canlı takipten veri gelmiyor.')
      }
      if (signal.aborted || isAbortError(err)) return 'stopped'
      return err instanceof ApiError
        ? err
        : new ApiError(0, 'INTERNAL', 'Beklenmeyen bir hata oluştu.')
    } finally {
      clearTimeout(watchdog)
      signal.removeEventListener('abort', abortAttempt)
    }
  }

  /** Elle yeniden bağlanır (failed durumundaki "Yeniden bağlan"). */
  function retry() {
    start(campaignId())
  }

  watch(campaignId, start, { immediate: true })
  onBeforeUnmount(stop)

  return { snapshot, connection, failures, error, updatedAt, retry }
}

/** Verilen süre kadar bekler; beklerken iptal edilirse false döner. */
function sleep(ms: number, signal: AbortSignal): Promise<boolean> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve(false)
      return
    }
    const onAbort = () => {
      clearTimeout(timer)
      resolve(false)
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve(true)
    }, ms)
    signal.addEventListener('abort', onAbort, { once: true })
  })
}
