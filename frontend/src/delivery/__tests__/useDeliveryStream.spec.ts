import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { errorResponse, controlledSse, sseEvent, type ControlledSse } from '@/__tests__/helpers'
import { useDeliveryStream, type DeliveryStreamOptions } from '../useDeliveryStream'

type Stream = ReturnType<typeof useDeliveryStream>

const fetchMock = vi.fn<typeof fetch>()

interface Connection {
  url: string
  signal: AbortSignal | null | undefined
  sse: ControlledSse
}
let connections: Connection[] = []
let mounted: VueWrapper[] = []

beforeEach(() => {
  vi.useFakeTimers()
  connections = []
  mounted = []
  fetchMock.mockReset()
  // Varsayılan: her istek, testin sonradan yönettiği yeni bir SSE bağlantısıdır.
  fetchMock.mockImplementation(async (input, init) => {
    const sse = controlledSse(init?.signal)
    connections.push({ url: String(input), signal: init?.signal, sse })
    return sse.response
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  mounted.forEach((w) => w.unmount())
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

/** Bekleyen mikro görevlerin (akış okumaları, promise zincirleri) bitmesini sağlar. */
const settle = async () => {
  await vi.advanceTimersByTimeAsync(0)
  await vi.advanceTimersByTimeAsync(0)
}

function conn(index: number): Connection {
  const found = connections[index]
  if (!found) throw new Error(`${index}. bağlantı yok (toplam ${connections.length})`)
  return found
}

function mountStream(initial: string | null, options?: DeliveryStreamOptions) {
  const id = ref<string | null>(initial)
  const captured: Stream[] = []
  const Host = defineComponent({
    setup() {
      captured.push(useDeliveryStream(() => id.value, options))
      return () => h('div')
    },
  })
  const wrapper = mount(Host)
  mounted.push(wrapper)
  const stream = captured[0]
  if (!stream) throw new Error('useDeliveryStream çalışmadı')
  return { id, wrapper, stream }
}

const counts = (sent: number, extra: Record<string, number> = {}) => ({
  queued: 0,
  retrying: 0,
  sending: 0,
  sent,
  failed: 0,
  skipped: 0,
  ...extra,
})
const snap = (sent: number, status = 'sending', failed = 0) =>
  sseEvent('snapshot', {
    status,
    messages: counts(sent, { failed }),
    failures: { exhausted: 0, rejected: 0 },
  })

const FAST: DeliveryStreamOptions = { retryDelayMs: () => 1000, maxFailures: 3 }

describe('bağlanma', () => {
  it('izlenecek kampanya yoksa bağlanmaz', async () => {
    const { stream } = mountStream(null)
    await settle()

    expect(stream.connection.value).toBe('idle')
    expect(stream.snapshot.value).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('kampanyaya bağlanır; ilk görüntüye kadar connecting, sonra live', async () => {
    const { stream } = mountStream('A')
    await settle()

    expect(stream.connection.value).toBe('connecting')
    expect(conn(0).url).toBe('/api/campaigns/A/events')
    expect(stream.snapshot.value).toBeNull()

    conn(0).sse.push(snap(5))
    await settle()

    expect(stream.connection.value).toBe('live')
    expect(stream.snapshot.value?.messages.sent).toBe(5)
    expect(stream.updatedAt.value).toBeInstanceOf(Date)
    expect(stream.error.value).toBeNull()
  })

  it('her yeni görüntü öncekinin yerini alır (üzerine eklenmez)', async () => {
    const { stream } = mountStream('A')
    await settle()
    conn(0).sse.push(snap(5))
    await settle()
    conn(0).sse.push(snap(7, 'sending', 1))
    await settle()

    expect(stream.snapshot.value?.messages.sent).toBe(7)
    expect(stream.snapshot.value?.messages.failed).toBe(1)
  })
})

describe('kopma ve yeniden bağlanma', () => {
  it('ağ koparsa son bilinen durumu korur, bekleyip yeniden bağlanır ve görüntü yenilenir', async () => {
    const { stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.push(snap(5))
    await settle()

    conn(0).sse.fail(new TypeError('network error'))
    await settle()
    expect(stream.connection.value).toBe('reconnecting')
    expect(stream.failures.value).toBe(1)
    expect(stream.snapshot.value?.messages.sent).toBe(5) // son bilinen durum duruyor

    await vi.advanceTimersByTimeAsync(999)
    expect(fetchMock).toHaveBeenCalledTimes(1) // bekleme dolmadan yeni istek yok
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(conn(1).url).toBe('/api/campaigns/A/events')

    // Yeni bağlantı güncel görüntüyle başlar: sayılar değişir, önceki sayıların üstüne eklenmez.
    conn(1).sse.push(snap(9))
    await settle()
    expect(stream.connection.value).toBe('live')
    expect(stream.failures.value).toBe(0)
    expect(stream.snapshot.value?.messages.sent).toBe(9)
  })

  it('sunucu akışı kapatırsa (tamamlanmadan) bunu kopma sayar ve yeniden bağlanır', async () => {
    const { stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.push(snap(5))
    await settle()
    conn(0).sse.close()
    await settle()

    expect(stream.connection.value).toBe('reconnecting')
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('sunucunun error olayı yeniden denenebilir hatadır', async () => {
    const { stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.push(sseEvent('error', { code: 'INTERNAL', message: 'x' }))
    await settle()

    expect(stream.connection.value).toBe('reconnecting')
    expect(stream.failures.value).toBe(1)
  })

  it('geçersiz görüntü ekrana ulaşmaz, bağlantı yenilenir', async () => {
    const { stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.push(snap(5))
    conn(0).sse.push(sseEvent('snapshot', { status: 'sending' })) // geçersiz
    await settle()

    expect(stream.snapshot.value?.messages.sent).toBe(5) // geçersiz olan yok sayıldı
    expect(stream.connection.value).toBe('reconnecting')
  })

  it('varsayılan bekleme üsteldir: 1, 2, 4, 8 sn; 5. ardışık başarısızlıkta vazgeçer', async () => {
    const { stream } = mountStream('A')
    await settle()

    for (const [index, wait] of [1000, 2000, 4000, 8000].entries()) {
      conn(index).sse.fail(new TypeError('network error'))
      await settle()
      expect(stream.connection.value).toBe('reconnecting')
      await vi.advanceTimersByTimeAsync(wait - 1)
      expect(fetchMock).toHaveBeenCalledTimes(index + 1) // bekleme dolmadan bağlanmaz
      await vi.advanceTimersByTimeAsync(1)
      expect(fetchMock).toHaveBeenCalledTimes(index + 2)
    }

    conn(4).sse.fail(new TypeError('network error')) // 5. ardışık başarısızlık
    await settle()
    expect(stream.connection.value).toBe('failed')
    expect(stream.error.value).toMatchObject({ code: 'STREAM_INTERRUPTED' })
  })

  it('görüntü alındıkça ardışık başarısızlık sayacı sıfırlanır: sürekli kopup toparlanan bağlantı pes etmez', async () => {
    const { stream } = mountStream('A', { retryDelayMs: () => 10, maxFailures: 2 })
    await settle()

    for (let i = 0; i < 6; i++) {
      conn(i).sse.push(snap(i + 1)) // her bağlantı bir görüntü verip kopuyor
      await settle()
      conn(i).sse.fail(new TypeError('network error'))
      await settle()
      expect(stream.connection.value).toBe('reconnecting')
      await vi.advanceTimersByTimeAsync(10)
    }
    expect(stream.connection.value).not.toBe('failed')
    expect(fetchMock).toHaveBeenCalledTimes(7)
  })

  it('üst üste başarısız olursa vazgeçer; elle yeniden bağlanınca düzelir', async () => {
    const { stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.push(snap(5))
    await settle()

    for (let i = 0; i < 3; i++) {
      conn(i).sse.fail(new TypeError('network error'))
      await settle()
      if (i < 2) await vi.advanceTimersByTimeAsync(1000)
    }
    expect(stream.connection.value).toBe('failed')
    expect(stream.error.value).toMatchObject({ code: 'STREAM_INTERRUPTED' })
    expect(stream.snapshot.value?.messages.sent).toBe(5) // son bilinen durum korunur

    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetchMock).toHaveBeenCalledTimes(3) // vazgeçtikten sonra kendiliğinden denemez

    stream.retry()
    await settle()
    expect(stream.connection.value).toBe('connecting')
    expect(stream.error.value).toBeNull()
    expect(stream.failures.value).toBe(0)
    expect(fetchMock).toHaveBeenCalledTimes(4)
    conn(3).sse.push(snap(11))
    await settle()
    expect(stream.connection.value).toBe('live')
    expect(stream.snapshot.value?.messages.sent).toBe(11)
  })

  it('yeniden denemeyle düzelmeyecek hatada (kampanya yok) hemen vazgeçer', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))
    const { stream } = mountStream('A', FAST)
    await settle()

    expect(stream.connection.value).toBe('failed')
    expect(stream.error.value).toMatchObject({ code: 'CAMPAIGN_NOT_FOUND', status: 404 })
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('ağ hatasında (sunucu kapalı) yeniden dener', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    const { stream } = mountStream('A', FAST)
    await settle()

    expect(stream.connection.value).toBe('reconnecting')
    expect(stream.failures.value).toBe(1)
    await vi.advanceTimersByTimeAsync(1000)
    conn(0).sse.push(snap(1))
    await settle()
    expect(stream.connection.value).toBe('live')
  })
})

describe('tamamlanma', () => {
  it('tamamlanmış görüntüde canlı takibi bilerek kapatır ve bir daha bağlanmaz', async () => {
    const { stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.push(snap(5))
    await settle()
    conn(0).sse.push(snap(9, 'completed', 1))
    await settle()

    expect(stream.connection.value).toBe('completed')
    expect(stream.snapshot.value?.status).toBe('completed')
    expect(stream.snapshot.value?.messages.sent).toBe(9)
    expect(stream.error.value).toBeNull()
    expect(conn(0).signal?.aborted).toBe(true) // bağlantı kapatıldı

    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetchMock).toHaveBeenCalledTimes(1) // yeniden bağlanma yok
    expect(stream.connection.value).toBe('completed')
  })

  it('tamamlanma, bağlantının kapanması beklenmeden hemen yansır (iptal geç ulaşsa bile)', async () => {
    // İptali dinlemeyen bir akış: sunucu bağlantıyı açık tutuyor ve iptal ağda gecikiyor.
    let open: ReadableStreamDefaultController<Uint8Array> | undefined
    fetchMock.mockResolvedValueOnce(
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            open = controller
          },
        }),
        { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
      ),
    )
    const { stream } = mountStream('A', { stallTimeoutMs: 5000 })
    await settle()

    open?.enqueue(new TextEncoder().encode(snap(7, 'completed')))
    await settle()

    expect(stream.connection.value).toBe('completed')
    expect(stream.snapshot.value?.messages.sent).toBe(7)
    await vi.advanceTimersByTimeAsync(60_000) // takılma izleyicisi de durumu bozmaz
    expect(stream.connection.value).toBe('completed')
    expect(stream.error.value).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('ilk görüntü zaten tamamlanmışsa da kapatır', async () => {
    const { stream } = mountStream('A')
    await settle()
    conn(0).sse.push(snap(3, 'completed'))
    await settle()

    expect(stream.connection.value).toBe('completed')
    expect(stream.snapshot.value?.messages.sent).toBe(3)
  })
})

describe('kampanya değişimi ve temizlik', () => {
  it('kampanya değişince eski bağlantı iptal edilir, eski sayılar silinir, yenisine bağlanılır', async () => {
    const { id, stream } = mountStream('A')
    await settle()
    conn(0).sse.push(snap(5))
    await settle()
    expect(stream.snapshot.value?.messages.sent).toBe(5)

    id.value = 'B'
    await settle()

    expect(conn(0).signal?.aborted).toBe(true)
    expect(stream.snapshot.value).toBeNull() // A'nın sayıları B'de görünmez
    expect(stream.connection.value).toBe('connecting')
    expect(conn(1).url).toBe('/api/campaigns/B/events')

    conn(1).sse.push(snap(2))
    await settle()
    expect(stream.snapshot.value?.messages.sent).toBe(2)
  })

  it('eski kampanyadan sonradan gelen olaylar ve hatalar yeni kampanyayı değiştirmez', async () => {
    // Eski bağlantı iptal edilse bile veri yollamaya devam eden bir akış (geç gelen olay).
    let late: ReadableStreamDefaultController<Uint8Array> | undefined
    const lateStream = new ReadableStream<Uint8Array>({
      start(controller) {
        late = controller
      },
    })
    fetchMock.mockResolvedValueOnce(
      new Response(lateStream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }),
    )
    const encoder = new TextEncoder()

    const { id, stream } = mountStream('A')
    await settle()
    id.value = 'B' // A'nın bağlantısı iptal sayılır ama akışı hâlâ canlı
    await settle()
    conn(0).sse.push(snap(2)) // B'nin ilk görüntüsü (bağlantı 0 = B)
    await settle()
    expect(stream.snapshot.value?.messages.sent).toBe(2)

    late?.enqueue(encoder.encode(snap(99, 'completed'))) // A'dan geç gelen olay
    await settle()
    expect(stream.snapshot.value?.messages.sent).toBe(2)
    expect(stream.snapshot.value?.status).toBe('sending')
    expect(stream.connection.value).toBe('live')

    late?.error(new TypeError('eski bağlantı koptu')) // A'nın hatası B'yi bozmaz
    await settle()
    // Takılma eşiğinden (8 sn) kısa bir süre: B'nin kendi sessizliği karışmasın.
    await vi.advanceTimersByTimeAsync(3000)
    expect(stream.connection.value).toBe('live')
    expect(stream.error.value).toBeNull()
    expect(stream.failures.value).toBe(0)
    expect(fetchMock).toHaveBeenCalledTimes(2) // A'nın kopması yeniden bağlanma başlatmadı
  })

  it('kampanya kalmayınca (null) bağlantıyı kapatır ve sıfırlar', async () => {
    const { id, stream } = mountStream('A')
    await settle()
    conn(0).sse.push(snap(5))
    await settle()

    id.value = null
    await settle()

    expect(conn(0).signal?.aborted).toBe(true)
    expect(stream.connection.value).toBe('idle')
    expect(stream.snapshot.value).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('bileşen kaldırılınca bağlantı kapanır', async () => {
    const { wrapper } = mountStream('A')
    await settle()
    wrapper.unmount()
    await settle()

    expect(conn(0).signal?.aborted).toBe(true)
  })

  it('bileşen yeniden bağlanma beklerken kaldırılırsa bir daha bağlanmaz', async () => {
    const { wrapper, stream } = mountStream('A', FAST)
    await settle()
    conn(0).sse.fail(new TypeError('network error'))
    await settle()
    expect(stream.connection.value).toBe('reconnecting')

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('kampanya değişimi yeniden bağlanma beklerken eski beklemeyi iptal eder', async () => {
    const { id } = mountStream('A', FAST)
    await settle()
    conn(0).sse.fail(new TypeError('network error')) // A için 1 sn bekleme başladı
    await settle()

    id.value = 'B'
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(conn(1).url).toBe('/api/campaigns/B/events')

    await vi.advanceTimersByTimeAsync(5000)
    expect(fetchMock).toHaveBeenCalledTimes(2) // eski bekleme A'ya bir daha bağlanmadı
  })
})

describe('takılan bağlantı', () => {
  const STALL: DeliveryStreamOptions = {
    stallTimeoutMs: 5000,
    retryDelayMs: () => 1000,
    maxFailures: 3,
  }

  it('görüntü gelmeyen bağlantıyı takılmış sayıp yeniden bağlanır; son bilinen durum korunur', async () => {
    const { stream } = mountStream('A', STALL)
    await settle()
    conn(0).sse.push(snap(5))
    await settle()

    await vi.advanceTimersByTimeAsync(4999)
    expect(stream.connection.value).toBe('live')
    await vi.advanceTimersByTimeAsync(1)

    expect(conn(0).signal?.aborted).toBe(true)
    expect(stream.connection.value).toBe('reconnecting')
    expect(stream.failures.value).toBe(1)
    expect(stream.snapshot.value?.messages.sent).toBe(5)

    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    conn(1).sse.push(snap(8))
    await settle()
    expect(stream.connection.value).toBe('live')
    expect(stream.snapshot.value?.messages.sent).toBe(8)
  })

  it('düzenli görüntü geldikçe takılma sayılmaz', async () => {
    const { stream } = mountStream('A', STALL)
    await settle()
    for (let i = 1; i <= 10; i++) {
      conn(0).sse.push(snap(i))
      await settle()
      await vi.advanceTimersByTimeAsync(3000) // toplam 30 sn, her aralık eşikten kısa
    }

    expect(stream.connection.value).toBe('live')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('bağlantı kurulurken de yanıt gelmezse (başlıklar gelmedi) takılmış sayılır', async () => {
    fetchMock.mockImplementationOnce(
      (_input, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () =>
            reject(new DOMException('Aborted', 'AbortError')),
          )
        }),
    )
    const { stream } = mountStream('A', STALL)
    await settle()
    expect(stream.connection.value).toBe('connecting')

    await vi.advanceTimersByTimeAsync(5000)
    expect(stream.connection.value).toBe('reconnecting')
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('tamamlanma sonrası takılma izleyicisi çalışmaz', async () => {
    const { stream } = mountStream('A', STALL)
    await settle()
    conn(0).sse.push(snap(5, 'completed'))
    await settle()
    await vi.advanceTimersByTimeAsync(60_000)

    expect(stream.connection.value).toBe('completed')
    expect(stream.error.value).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
