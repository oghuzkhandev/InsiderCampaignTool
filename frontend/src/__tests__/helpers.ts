import type { Campaign } from '@/api/campaigns'

export function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

export function errorResponse(
  status: number,
  code: string,
  reasons: { code: string; field?: string; text?: string }[] = [],
): Response {
  return jsonResponse(status, { error: { code, message: 'Sunucu mesajı', reasons } })
}

export function campaignBody(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: '498882b6-2221-4589-a99f-67ceb68c59c2',
    brief: 'Örnek brief',
    status: 'draft',
    segment: {
      all: [
        { field: 'city', op: 'eq', value: 'İstanbul' },
        { field: 'last_cart_add_at', op: 'within_last_days', value: 30 },
        { field: 'last_purchase_at', op: 'not_within_last_days', value: 30 },
      ],
    },
    audience: { total: 190, sms: 140, email: 150 },
    created_at: '2026-11-17T17:57:48.943702Z',
    ...overrides,
  }
}

export interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

export function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
