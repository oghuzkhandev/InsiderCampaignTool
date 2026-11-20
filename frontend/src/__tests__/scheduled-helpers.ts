import type { ScheduledCampaign, ScheduledPage } from '@/api/scheduled'

// "Zamanlanmış gönderimler" testleri için ortak yardımcılar.

export function scheduledRow(over: Partial<ScheduledCampaign> = {}): ScheduledCampaign {
  return {
    id: 'c1',
    brief: 'İstanbul sepet hatırlatması',
    scheduled_at: '2026-11-21T10:30:00Z',
    approved_at: '2026-11-21T08:00:00Z',
    queued: 191,
    due: false,
    ...over,
  }
}

export function scheduledPageBody(over: Partial<ScheduledPage> = {}): ScheduledPage {
  const items = over.items ?? [scheduledRow()]
  return {
    items,
    total: items.length,
    page: 1,
    limit: 10,
    server_time: '2026-11-21T09:00:00Z',
    ...over,
  }
}
