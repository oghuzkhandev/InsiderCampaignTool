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

// --- "Segmente uyan kişiler" listesi testleri için yardımcılar ---

import type { AudienceMember, AudiencePage } from '@/api/audience'

export function memberBody(overrides: Partial<AudienceMember> = {}): AudienceMember {
  return {
    id: 1,
    first_name: 'Ayşe',
    city: 'İstanbul',
    email: 'ayse@example.test',
    phone: '+905550000001',
    sms_status: 'eligible',
    email_status: 'eligible',
    ...overrides,
  }
}

/** `items` verilmezse tek kişilik bir sayfa; `total` verilmezse sayfadaki kişi sayısı. */
export function audiencePageBody(overrides: Partial<AudiencePage> = {}): AudiencePage {
  const items = overrides.items ?? [memberBody()]
  return { page: 1, limit: 10, total: items.length, items, ...overrides }
}

/** `count` kişilik bir sayfa; kimlikler `firstId`'den başlar. */
export function members(count: number, firstId = 1): AudienceMember[] {
  return Array.from({ length: count }, (_, i) =>
    memberBody({
      id: firstId + i,
      first_name: `Kişi ${firstId + i}`,
      email: `kisi${firstId + i}@example.test`,
    }),
  )
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

// --- İçerik ve onay testleri için yardımcılar ---

import type { ContentVersion } from '@/api/content'

export function contentBody(overrides: Partial<ContentVersion> = {}): ContentVersion {
  return {
    content_version: 1,
    source: 'llm',
    sms_text: 'Merhaba {{first_name}}, %30 indirim seni bekliyor!',
    email_subject: 'Sepetin seni bekliyor',
    email_body: 'Merhaba {{first_name}},\n\nÜrünlerin hâlâ burada.',
    created_at: '2026-11-18T10:00:00Z',
    ...overrides,
  }
}

/** Backend'in yazdığı biçimde tek bir SSE olayı. */
export function sseEvent(name: string, data: unknown): string {
  return `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`
}

const encoder = new TextEncoder()

/** Verilen parçaları sırayla yayınlayıp kapanan bir SSE yanıtı. */
export function sseResponse(chunks: (string | Uint8Array)[], status = 200): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(typeof chunk === 'string' ? encoder.encode(chunk) : chunk)
      }
      controller.close()
    },
  })
  return new Response(stream, { status, headers: { 'Content-Type': 'text/event-stream' } })
}

export interface ControlledSse {
  response: Response
  push: (text: string) => void
  close: () => void
  fail: (error: unknown) => void
  readonly cancelled: boolean
}

/**
 * Parçaları testin istediği anda yayınlayan SSE yanıtı. İstek iptal edilirse
 * (signal) akış AbortError ile kopar.
 */
export function controlledSse(signal?: AbortSignal | null): ControlledSse {
  let controller!: ReadableStreamDefaultController<Uint8Array>
  let cancelled = false
  const stream = new ReadableStream<Uint8Array>({
    start(c) {
      controller = c
    },
    cancel() {
      cancelled = true
    },
  })
  signal?.addEventListener('abort', () => {
    try {
      controller.error(new DOMException('Aborted', 'AbortError'))
    } catch {
      /* akış zaten kapalı */
    }
  })
  return {
    response: new Response(stream, {
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
    }),
    push: (text) => controller.enqueue(encoder.encode(text)),
    close: () => {
      try {
        controller.close()
      } catch {
        /* istemci akışı zaten iptal etti */
      }
    },
    fail: (error) => controller.error(error),
    get cancelled() {
      return cancelled
    },
  }
}
