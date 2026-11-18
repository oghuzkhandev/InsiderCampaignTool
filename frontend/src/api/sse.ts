// Server-Sent Events ayrıştırıcısı. Backend her olayı `event: ad` ve tek
// satırlık `data: json` ile yollar, boş satırla bitirir (docs/ARCHITECTURE.md §5).
// Parça sınırları herhangi bir yerde olabilir (satırın, \r\n çiftinin veya
// çok baytlı bir karakterin ortası dahil).

export interface SseEvent {
  event: string
  data: string
}

/** Artımlı SSE ayrıştırıcısı: metin parçalarını verir, tamamlanan olayları döndürür. */
export class SseParser {
  private buffer = ''

  push(text: string): SseEvent[] {
    this.buffer += text
    // Sonda tek başına duran \r, bir sonraki parçadaki \n ile \r\n olabilir.
    let ready = this.buffer
    let pending = ''
    if (ready.endsWith('\r')) {
      pending = '\r'
      ready = ready.slice(0, -1)
    }
    ready = ready.replace(/\r\n?/g, '\n')

    const events: SseEvent[] = []
    let start = 0
    for (;;) {
      const end = ready.indexOf('\n\n', start)
      if (end === -1) break
      const event = parseBlock(ready.slice(start, end))
      if (event) events.push(event)
      start = end + 2
    }
    this.buffer = ready.slice(start) + pending
    return events
  }

  /**
   * Akış bittiğinde çağrılır: sonda bekleyen tek \r satır sonu sayılır. Boş
   * satırla bitmemiş (tamamlanmamış) olay verilmez.
   */
  end(): SseEvent[] {
    const rest = this.buffer
    this.buffer = ''
    return rest.endsWith('\r') ? new SseParser().push(rest + '\n') : []
  }
}

function parseBlock(block: string): SseEvent | null {
  let event = 'message'
  const data: string[] = []
  let hasField = false
  for (const line of block.split('\n')) {
    if (line === '' || line.startsWith(':')) continue // yorum (ör. keepalive)
    const colon = line.indexOf(':')
    const field = colon === -1 ? line : line.slice(0, colon)
    let value = colon === -1 ? '' : line.slice(colon + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'event') {
      event = value
      hasField = true
    } else if (field === 'data') {
      data.push(value)
      hasField = true
    }
  }
  if (!hasField) return null
  return { event, data: data.join('\n') }
}

/**
 * Yanıt gövdesini okuyup olayları sırayla verir. Tüketici döngüden çıkarsa
 * (return, throw) akış iptal edilir.
 */
export async function* readSse(body: ReadableStream<Uint8Array>): AsyncGenerator<SseEvent> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  const parser = new SseParser()
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      for (const event of parser.push(decoder.decode(value, { stream: true }))) yield event
    }
    for (const event of parser.push(decoder.decode())) yield event
    for (const event of parser.end()) yield event
  } finally {
    // İptal hatası yok sayılır; akış zaten bitmiş veya koparılmış olabilir.
    await reader.cancel().catch(() => undefined)
  }
}
