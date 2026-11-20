import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { toast } from 'vue-sonner'
import NotificationToast from '../NotificationToast.vue'
import type { NotifyKind } from '../types'

vi.mock('vue-sonner', () => ({
  toast: { dismiss: vi.fn<(id?: string | number) => void>(), custom: vi.fn<() => string>() },
}))

beforeEach(() => {
  vi.mocked(toast.dismiss).mockClear()
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
  vi.setSystemTime(new Date('2026-11-21T09:00:00Z'))
})
afterEach(() => {
  vi.useRealTimers()
})

function mountToast(
  props: Partial<{
    kind: NotifyKind
    title: string
    description: string
    durationMs: number
    id: string
  }> = {},
) {
  return mount(NotificationToast, {
    props: { id: 'n1', kind: 'success', title: 'Kaydedildi', durationMs: 12_000, ...props },
  })
}

const time = (w: ReturnType<typeof mountToast>) => w.get('[data-testid="notification-time"]').text()
const bar = (w: ReturnType<typeof mountToast>) =>
  (w.get('[data-testid="notification-bar"]').element as HTMLElement).style.width

describe('NotificationToast', () => {
  describe('görünüm', () => {
    it('başlığı ve açıklamayı gösterir; açıklama yoksa öğe çıkmaz', () => {
      const w = mountToast({ title: 'Şablon kaydedildi', description: 'Kullanabilirsiniz.' })

      expect(w.get('[data-testid="notification-title"]').text()).toBe('Şablon kaydedildi')
      expect(w.get('[data-testid="notification-description"]').text()).toBe('Kullanabilirsiniz.')
      expect(mountToast().find('[data-testid="notification-description"]').exists()).toBe(false)
    })

    it.each([
      ['success', 'status'],
      ['info', 'status'],
      ['warning', 'alert'],
      ['error', 'alert'],
    ] as const)(
      '%s türü %s rolüyle okunur (hata ve uyarı hemen, diğerleri nazikçe)',
      (kind, role) => {
        const w = mountToast({ kind })

        expect(w.attributes('role')).toBe(role)
        expect(w.attributes('data-kind')).toBe(kind)
      },
    )

    it('dört tür dört ayrı ikon ve ton kullanır; ikon ekran okuyucudan gizlidir', () => {
      const looks = (['success', 'warning', 'error', 'info'] as const).map((kind) => {
        const w = mountToast({ kind })
        const tile = w.get('span[aria-hidden="true"]')
        return `${tile.get('svg').html()}|${tile.classes().join(' ')}`
      })

      expect(new Set(looks).size).toBe(4)
    })

    it('metni HTML olarak yorumlamaz', () => {
      const w = mountToast({ title: '<b>kalın</b>', description: '<img src=x onerror=alert(1)>' })

      expect(w.find('b').exists()).toBe(false)
      expect(w.find('img').exists()).toBe(false)
      expect(w.get('[data-testid="notification-title"]').text()).toBe('<b>kalın</b>')
    })

    it('kalan süre ve çubuk ekran okuyucudan gizlidir (her saniye okunmaz)', () => {
      const w = mountToast()

      expect(w.get('[data-testid="notification-time"]').attributes('aria-hidden')).toBe('true')
      expect(
        w
          .get('[data-testid="notification-bar"]')
          .element.parentElement?.getAttribute('aria-hidden'),
      ).toBe('true')
    })

    it('kapatma düğmesinin erişilebilir adı Türkçedir', () => {
      expect(mountToast().get('[data-testid="notification-close"]').attributes('aria-label')).toBe(
        'Bildirimi kapat',
      )
    })
  })

  describe('geri sayım', () => {
    it('12 sn ile başlar ve çubuk tamdır', () => {
      const w = mountToast()

      expect(time(w)).toBe('12 sn')
      expect(bar(w)).toBe('100%')
    })

    it('süre aktıkça sayı ve çubuk azalır', async () => {
      const w = mountToast()

      await vi.advanceTimersByTimeAsync(3_000)
      expect(time(w)).toBe('9 sn')
      expect(bar(w)).toBe('75%')

      await vi.advanceTimersByTimeAsync(6_000)
      expect(time(w)).toBe('3 sn')
      expect(bar(w)).toBe('25%')
    })

    it('sayı yukarı yuvarlanır: son saniyede "1 sn" görünür, 0 görünmez', async () => {
      const w = mountToast()

      await vi.advanceTimersByTimeAsync(11_500)

      expect(time(w)).toBe('1 sn')
    })

    it('süre dolunca kendini kapatır (bir kez)', async () => {
      mountToast({ id: 'abc' })

      await vi.advanceTimersByTimeAsync(11_900)
      expect(toast.dismiss).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(300)

      expect(toast.dismiss).toHaveBeenCalledTimes(1)
      expect(toast.dismiss).toHaveBeenCalledWith('abc')
      await vi.advanceTimersByTimeAsync(5_000)
      expect(toast.dismiss).toHaveBeenCalledTimes(1) // sayaç durdu
    })

    it('özel süreyi kullanır', async () => {
      const w = mountToast({ durationMs: 4_000 })
      expect(time(w)).toBe('4 sn')

      await vi.advanceTimersByTimeAsync(4_100)

      expect(toast.dismiss).toHaveBeenCalledTimes(1)
    })
  })

  describe('duraklatma', () => {
    it('imleç üzerindeyken süre akmaz ve "Duraklatıldı" yazar; ayrılınca kaldığı yerden sürer', async () => {
      const w = mountToast()
      await vi.advanceTimersByTimeAsync(3_000)

      await w.trigger('mouseenter')
      await vi.advanceTimersByTimeAsync(20_000) // çok uzun beklense de kapanmaz
      expect(time(w)).toBe('Duraklatıldı')
      expect(w.attributes('data-paused')).toBe('true')
      expect(toast.dismiss).not.toHaveBeenCalled()

      await w.trigger('mouseleave')
      expect(time(w)).toBe('9 sn') // duraklama süresi sayılmadı
      await vi.advanceTimersByTimeAsync(9_100)
      expect(toast.dismiss).toHaveBeenCalledTimes(1)
    })

    it('odak içerideyken (klavye) de duraklar', async () => {
      const w = mountToast()

      await w.get('[data-testid="notification-close"]').trigger('focusin')
      await vi.advanceTimersByTimeAsync(15_000)

      expect(toast.dismiss).not.toHaveBeenCalled()
      expect(time(w)).toBe('Duraklatıldı')

      await w.trigger('focusout')
      await vi.advanceTimersByTimeAsync(12_100)
      expect(toast.dismiss).toHaveBeenCalledTimes(1)
    })
  })

  describe('kapatma', () => {
    it('kapatma düğmesi bildirimi hemen kapatır ve sayacı durdurur', async () => {
      const w = mountToast({ id: 'x9' })

      await w.get('[data-testid="notification-close"]').trigger('click')

      expect(toast.dismiss).toHaveBeenCalledWith('x9')
      await vi.advanceTimersByTimeAsync(20_000)
      expect(toast.dismiss).toHaveBeenCalledTimes(1)
    })

    it('bileşen kaldırılınca sayaç durur (sızıntı yok)', async () => {
      const w = mountToast()

      w.unmount()
      await vi.advanceTimersByTimeAsync(20_000)

      expect(toast.dismiss).not.toHaveBeenCalled()
    })
  })
})
