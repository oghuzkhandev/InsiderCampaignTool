import { describe, expect, it } from 'vitest'
import { SseParser, readSse } from '../sse'

const sample =
  'event: delta\ndata: {"field":"sms_text","text":"Merhaba"}\n\n' +
  'event: validated\ndata: {"content_version":1}\n\n'
const expected = [
  { event: 'delta', data: '{"field":"sms_text","text":"Merhaba"}' },
  { event: 'validated', data: '{"content_version":1}' },
]

function parseAll(...chunks: string[]) {
  const p = new SseParser()
  return [...chunks.flatMap((c) => p.push(c)), ...p.end()]
}

describe('SseParser', () => {
  it('birden çok olayı tek parçada ayrıştırır', () => {
    expect(parseAll(sample)).toEqual(expected)
  })

  it('parça sınırı her yerde olsa da aynı olayları verir', () => {
    for (let i = 0; i <= sample.length; i++) {
      expect(parseAll(sample.slice(0, i), sample.slice(i))).toEqual(expected)
    }
    expect(parseAll(...sample.split(''))).toEqual(expected)
  })

  it('tamamlanmamış olayı boş satır gelene kadar vermez', () => {
    const p = new SseParser()
    expect(p.push('event: delta\ndata: {"a":1}\n')).toEqual([])
    expect(p.push('\n')).toEqual([{ event: 'delta', data: '{"a":1}' }])
  })

  it('CRLF ve tek başına CR satır sonlarını destekler, parça CR ile bitse de', () => {
    const crlf = sample.replace(/\n/g, '\r\n')
    expect(parseAll(crlf)).toEqual(expected)
    for (let i = 0; i <= crlf.length; i++) {
      expect(parseAll(crlf.slice(0, i), crlf.slice(i))).toEqual(expected)
    }
    expect(parseAll(sample.replace(/\n/g, '\r'))).toEqual(expected)
  })

  it('yorum satırlarını (keepalive) ve alansız blokları yok sayar', () => {
    expect(parseAll(': keepalive\n\n', ': x\n\n\n\n', sample)).toEqual(expected)
    expect(parseAll('foo: bar\n\n')).toEqual([])
  })

  it('çok satırlı data alanlarını \\n ile birleştirir ve yalnızca bir baştaki boşluğu atar', () => {
    expect(parseAll('event: x\ndata: a\ndata:  b\n\n')).toEqual([{ event: 'x', data: 'a\n b' }])
  })

  it('akış sonunda bekleyen tek CR satır sonu sayılır; tamamlanmamış olay verilmez', () => {
    const p = new SseParser()
    expect(p.push('event: x\rdata: 1\r\r')).toEqual([])
    expect(p.end()).toEqual([{ event: 'x', data: '1' }])
    expect(new SseParser().end()).toEqual([])
    const q = new SseParser()
    q.push('event: x\ndata: yarım')
    expect(q.end()).toEqual([])
  })

  it('olay adı yoksa message varsayılır', () => {
    expect(parseAll('data: merhaba\n\n')).toEqual([{ event: 'message', data: 'merhaba' }])
  })

  it('metindeki satır sonları (JSON kaçışlı) olayı bölmez', () => {
    const data = JSON.stringify({ text: 'a\n\nevent: error\ndata: x\n\n' })
    expect(parseAll(`event: delta\ndata: ${data}\n\n`)).toEqual([{ event: 'delta', data }])
  })
})

function streamOf(
  chunks: Uint8Array[],
  onCancel?: () => void,
  keepOpen = false,
): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(c) {
      chunks.forEach((chunk) => c.enqueue(chunk))
      if (!keepOpen) c.close()
    },
    cancel: onCancel,
  })
}

async function collect(body: ReadableStream<Uint8Array>) {
  const out = []
  for await (const e of readSse(body)) out.push(e)
  return out
}

describe('readSse', () => {
  it('çok baytlı karakterler parça sınırında bölünse de doğru çözülür', async () => {
    const text = 'event: delta\ndata: {"text":"Çağşü İıÖö 😀 %30"}\n\n'
    const bytes = new TextEncoder().encode(text)
    for (let i = 1; i < bytes.length; i++) {
      const events = await collect(streamOf([bytes.slice(0, i), bytes.slice(i)]))
      expect(events).toEqual([{ event: 'delta', data: '{"text":"Çağşü İıÖö 😀 %30"}' }])
    }
  })

  it('tüketici erken çıkarsa akışı iptal eder', async () => {
    let cancelled = false
    const body = streamOf(
      [new TextEncoder().encode(sample)],
      () => {
        cancelled = true
      },
      true, // akış hâlâ açık: tüketici erken çıkınca iptal edilmeli
    )
    for await (const event of readSse(body)) {
      expect(event.event).toBe('delta')
      break
    }
    expect(cancelled).toBe(true)
  })

  it('akış bitince tamamlanmamış son blok verilmez', async () => {
    const events = await collect(
      streamOf([new TextEncoder().encode(sample + 'event: delta\ndata: {')]),
    )
    expect(events).toEqual(expected)
  })
})
