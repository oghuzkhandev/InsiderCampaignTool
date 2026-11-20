import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import NotificationToast from '../NotificationToast.vue'
import { NOTIFY_DURATION_MS, notify } from '../notify'

vi.mock('vue-sonner', () => ({
  toast: { custom: vi.fn<() => string>(), dismiss: vi.fn<(id?: string | number) => void>() },
}))

beforeEach(() => {
  vi.mocked(toast.custom).mockReset()
  vi.mocked(toast.dismiss).mockReset()
})

function lastCall() {
  const calls = vi.mocked(toast.custom).mock.calls
  const [component, data] = calls[calls.length - 1]!
  return { component, data: data! }
}

describe('notify', () => {
  it('varsayılan süre 8 saniyedir', () => {
    expect(NOTIFY_DURATION_MS).toBe(8_000)
  })

  it.each(['success', 'warning', 'error', 'info'] as const)(
    '%s türünü bildirim kartıyla gösterir',
    (kind) => {
      notify[kind]('Başlık', 'Açıklama')

      const { component, data } = lastCall()
      expect(component).toBe(NotificationToast)
      expect(data.componentProps).toMatchObject({
        kind,
        title: 'Başlık',
        description: 'Açıklama',
        durationMs: 8_000,
      })
    },
  )

  it('ömrü kartın kendisi yönetir: sonner kendiliğinden kapatmasın diye süre sonsuzdur', () => {
    notify.success('Kaydedildi')

    expect(lastCall().data.duration).toBe(Number.POSITIVE_INFINITY)
  })

  it('her bildirime benzersiz kimlik verir ve aynı kimliği karta iletir', () => {
    const first = notify.success('A')
    const firstData = lastCall().data
    const second = notify.error('B')
    const secondData = lastCall().data

    expect(first).not.toBe(second)
    expect(firstData.id).toBe(first)
    expect(firstData.componentProps.id).toBe(first)
    expect(secondData.id).toBe(second)
  })

  it('açıklama isteğe bağlıdır', () => {
    notify.info('Sadece başlık')

    expect(lastCall().data.componentProps.description).toBeUndefined()
  })

  it('özel süre verilebilir', () => {
    notify.warning('Uyarı', 'Dikkat', { durationMs: 5_000 })

    expect(lastCall().data.componentProps.durationMs).toBe(5_000)
  })

  it('dismiss kimlikle kapatır', () => {
    notify.dismiss('notify-7')

    expect(toast.dismiss).toHaveBeenCalledWith('notify-7')
  })
})
