import { describe, expect, it } from 'vitest'
import { possessiveSuffix, withPossessive } from '../turkish'

describe('possessiveSuffix', () => {
  it('tek basamakları okunuşlarına göre eklendirir (bir, iki, üç, dört, beş, altı, yedi, sekiz, dokuz)', () => {
    const expected: Record<number, string> = {
      0: 'ı', // sıfırı
      1: 'i', // biri
      2: 'si', // ikisi
      3: 'ü', // üçü
      4: 'ü', // dördü
      5: 'i', // beşi
      6: 'sı', // altısı
      7: 'si', // yedisi
      8: 'i', // sekizi
      9: 'u', // dokuzu
    }
    for (const [n, suffix] of Object.entries(expected)) {
      expect(possessiveSuffix(Number(n)), `${n}`).toBe(suffix)
    }
  })

  it('onlukları okunuşlarına göre eklendirir (on, yirmi, otuz, kırk, elli, altmış, yetmiş, seksen, doksan)', () => {
    const expected: Record<number, string> = {
      10: 'u', // onu
      20: 'si', // yirmisi
      30: 'u', // otuzu
      40: 'ı', // kırkı
      50: 'si', // ellisi
      60: 'ı', // altmışı
      70: 'i', // yetmişi
      80: 'i', // sekseni
      90: 'ı', // doksanı
    }
    for (const [n, suffix] of Object.entries(expected)) {
      expect(possessiveSuffix(Number(n)), `${n}`).toBe(suffix)
    }
  })

  it('iki basamaklı sayıda son sözcüğe bakar', () => {
    expect(possessiveSuffix(12)).toBe('si') // on ikisi
    expect(possessiveSuffix(25)).toBe('i') // yirmi beşi
    expect(possessiveSuffix(47)).toBe('si') // kırk yedisi
    expect(possessiveSuffix(93)).toBe('ü') // doksan üçü
  })

  it('yüzler, binler ve milyonlar', () => {
    expect(possessiveSuffix(100)).toBe('ü') // yüzü
    expect(possessiveSuffix(300)).toBe('ü') // üç yüzü
    expect(possessiveSuffix(1000)).toBe('i') // bini
    expect(possessiveSuffix(5000)).toBe('i') // beş bini
    expect(possessiveSuffix(1_000_000)).toBe('u') // milyonu
    expect(possessiveSuffix(1_000_000_000)).toBe('ı') // milyarı
  })

  it('büyük sayılarda yine son sözcüğe bakar', () => {
    expect(possessiveSuffix(1234)).toBe('ü') // bin iki yüz otuz dördü
    expect(possessiveSuffix(1206)).toBe('sı') // bin iki yüz altısı
    expect(possessiveSuffix(1210)).toBe('u') // bin iki yüz onu
  })
})

describe('withPossessive', () => {
  it('sayıyı Türkçe biçimler ve kesme işaretiyle eki ekler', () => {
    expect(withPossessive(5)).toBe("5'i")
    expect(withPossessive(7)).toBe("7'si")
    expect(withPossessive(1)).toBe("1'i")
    expect(withPossessive(3)).toBe("3'ü")
  })

  it('binlik ayraç kullanır', () => {
    expect(withPossessive(1234)).toBe("1.234'ü")
    expect(withPossessive(12345)).toBe("12.345'i")
  })
})
