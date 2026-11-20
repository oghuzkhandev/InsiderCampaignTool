import type { Recipient, RecipientPage } from '@/api/recipients'

export function recipientBody(overrides: Partial<Recipient> = {}): Recipient {
  return {
    id: 1,
    user_id: 1,
    first_name: 'Ayşe',
    channel: 'sms',
    address: '+905550000001',
    status: 'sent',
    attempts: 1,
    sent_at: '2026-11-20T06:54:05Z',
    failure_reason: null,
    ...overrides,
  }
}

/** `items` verilmezse tek alıcılı bir sayfa; `total` verilmezse sayfadaki alıcı sayısı. */
export function recipientPageBody(overrides: Partial<RecipientPage> = {}): RecipientPage {
  const items = overrides.items ?? [recipientBody()]
  return { page: 1, limit: 10, total: items.length, items, ...overrides }
}

/** `count` alıcılı bir sayfa; kimlikler `firstId`'den başlar. */
export function recipients(count: number, firstId = 1): Recipient[] {
  return Array.from({ length: count }, (_, i) =>
    recipientBody({
      id: firstId + i,
      user_id: firstId + i,
      first_name: `Kişi ${firstId + i}`,
      address: `kisi${firstId + i}@example.test`,
      channel: 'email',
    }),
  )
}
