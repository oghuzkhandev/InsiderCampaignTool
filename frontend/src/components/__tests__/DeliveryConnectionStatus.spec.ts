import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DeliveryConnectionStatus from '../DeliveryConnectionStatus.vue'
import type { DeliveryConnection } from '@/delivery/useDeliveryStream'

interface Props {
  connection: DeliveryConnection
  failures?: number
  updatedAt?: Date | null
  errorTitle?: string | null
}

function mountStatus({ failures = 0, updatedAt = null, errorTitle = null, ...rest }: Props) {
  return mount(DeliveryConnectionStatus, { props: { failures, updatedAt, errorTitle, ...rest } })
}

// Yerel saat: biçimleme ortamdan bağımsız olsun diye bileşenle aynı yerelde üretilir.
const AT = new Date(2026, 10, 19, 14, 3, 7)
const AT_TEXT = new Intl.DateTimeFormat('tr-TR', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
}).format(AT)

describe('durumlar', () => {
  it('idle: hiçbir şey göstermez', () => {
    const w = mountStatus({ connection: 'idle' })
    expect(w.find('[data-testid="connection-status"]').exists()).toBe(false)
    expect(w.find('[data-testid="connection-failed"]').exists()).toBe(false)
  })

  it('connecting: bağlanılıyor der', () => {
    const w = mountStatus({ connection: 'connecting' })
    expect(w.get('[data-testid="connection-status"]').text()).toContain('Canlı takibe bağlanılıyor')
  })

  it('live: "Canlı" ve son güncelleme saatini gösterir', () => {
    const w = mountStatus({ connection: 'live', updatedAt: AT })
    const status = w.get('[data-testid="connection-status"]').text()

    expect(status).toContain('Canlı')
    expect(status).toContain(`Son güncelleme: ${AT_TEXT}`)
  })

  it('reconnecting: kopmayı, deneme sayısını ve sayıların son bilinen durum olduğunu söyler', () => {
    const w = mountStatus({ connection: 'reconnecting', failures: 2, updatedAt: AT })
    const status = w.get('[data-testid="connection-status"]').text()

    expect(status).toContain('Bağlantı koptu, yeniden bağlanılıyor (deneme 2)')
    expect(status).toContain('son bilinen durum')
    expect(status).toContain(AT_TEXT)
  })

  it('reconnecting: hiç görüntü gelmediyse saat göstermez', () => {
    const w = mountStatus({ connection: 'reconnecting', failures: 1 })
    expect(w.get('[data-testid="connection-status"]').text()).not.toMatch(/\d\d:\d\d:\d\d/)
  })

  it('completed: takibin bilerek kapatıldığını söyler', () => {
    const w = mountStatus({ connection: 'completed', updatedAt: AT })
    const status = w.get('[data-testid="connection-status"]').text()

    // Bağlantı rozeti kampanya durumundan ("Tamamlandı") ayrı bir sözcüktür.
    expect(status).toContain('Kapandı')
    expect(status).not.toContain('Tamamlandı')
    expect(status).toContain('Canlı takip kapatıldı')
    expect(status).toContain(AT_TEXT)
  })
})

describe('kalıcı hata', () => {
  it('hata başlığını gösterir ve "Yeniden bağlan" ile olay yollar', async () => {
    const w = mountStatus({
      connection: 'failed',
      errorTitle: 'Canlı takip bağlantısı koptu.',
      updatedAt: AT,
    })
    const alert = w.get('[data-testid="connection-failed"]')

    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toContain('Canlı takip bağlantısı koptu.')
    expect(alert.text()).toContain(`en son alınan durumdur (${AT_TEXT})`)

    await w.get('button').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  it('hiç görüntü gelmediyse "en son alınan durum" demez; başlık yoksa genel metin gösterir', () => {
    const w = mountStatus({ connection: 'failed' })
    const alert = w.get('[data-testid="connection-failed"]')

    expect(alert.text()).toContain('Canlı takip başlatılamadı.')
    expect(alert.text()).not.toContain('en son alınan durum')
    expect(w.find('[data-testid="connection-status"]').exists()).toBe(false)
  })

  it('hata durumunda yeniden bağlan dışında düğme yoktur', () => {
    const w = mountStatus({ connection: 'failed', errorTitle: 'x' })
    expect(w.findAll('button')).toHaveLength(1)
  })
})

describe('erişilebilirlik', () => {
  it('ekran okuyucuya bildirilen metin bağlantı durumuyla değişir', () => {
    const announce = (props: Props) =>
      mountStatus(props).get('[data-testid="connection-status"] [role="status"]').text()

    expect(announce({ connection: 'connecting' })).toBe('Canlı takibe bağlanılıyor.')
    expect(announce({ connection: 'live', updatedAt: AT })).toBe('Canlı takip bağlı.')
    expect(announce({ connection: 'reconnecting', failures: 3 })).toBe(
      'Bağlantı koptu, yeniden bağlanılıyor. Deneme 3.',
    )
    expect(announce({ connection: 'completed', updatedAt: AT })).toBe(
      'Gönderim tamamlandı. Canlı takip kapatıldı.',
    )
  })

  it('canlıyken saniyelik saat değişimi ekran okuyucuya okunmaz (canlı bölge saat içermez)', async () => {
    const w = mountStatus({ connection: 'live', updatedAt: AT })
    const region = w.get('[data-testid="connection-status"] [role="status"]')
    expect(region.text()).not.toMatch(/\d\d:\d\d:\d\d/)

    await w.setProps({ updatedAt: new Date(2026, 10, 19, 14, 3, 8) })
    expect(region.text()).toBe('Canlı takip bağlı.') // metin aynı kalır, yeniden okunmaz
  })

  it('görünür durum metni ekran okuyucudan gizlidir; bilgi yalnızca duyuru bölgesinden gelir (çift okunmaz)', () => {
    const states: DeliveryConnection[] = ['connecting', 'live', 'reconnecting', 'completed']
    for (const connection of states) {
      const w = mountStatus({ connection, failures: 1, updatedAt: AT })
      const visible = w.get('[data-testid="connection-status"] > div')

      expect(visible.attributes('aria-hidden')).toBe('true')
      expect(visible.text().length).toBeGreaterThan(0) // gözle görünen metin yine var
      // Duyuru bölgesi gizli bloğun içinde değil: ekran okuyucu onu okur.
      const live = w.get('[data-testid="connection-status"] [role="status"]')
      expect(visible.element.contains(live.element)).toBe(false)
    }
  })

  it('hata durumunda gizlenen bir şey yoktur (uyarı ve düğme erişilebilir)', () => {
    const w = mountStatus({ connection: 'failed', errorTitle: 'x' })
    // Yalnızca süs simgeleri gizlidir (aria-hidden svg); metin ve düğme gizli bir kapta değildir.
    const hidden = w.findAll('[aria-hidden="true"]')
    expect(hidden.every((el) => el.element.tagName.toLowerCase() === 'svg')).toBe(true)
    expect(w.find('[aria-hidden="true"] button').exists()).toBe(false)
    expect(w.get('[role="alert"]').text()).toContain('x')
  })
})
