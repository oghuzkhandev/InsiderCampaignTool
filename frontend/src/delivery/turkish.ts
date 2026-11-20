// Sayılara Türkçe iyelik eki ("5'i", "7'si", "3'ü"): ek, sayının OKUNUŞUNUN son hecesine göre
// seçilir. "5" beş okunur (ünsüzle biter, i) → "5'i"; "7" yedi okunur (ünlüyle biter) →
// "7'si". Ünlü uyumu ve ünlü/ünsüz bilgisi okunuşun son sözcüğünden gelir.

interface Ending {
  /** Okunuşun son sesi ünlü mü (ünlüyse araya "s" girer: yedi → yedisi). */
  vowel: boolean
  /** Son hecenin ünlüsüne göre ek ünlüsü. */
  harmony: 'i' | 'ı' | 'u' | 'ü'
}

// 0-9: sıfır, bir, iki, üç, dört, beş, altı, yedi, sekiz, dokuz
const DIGITS: readonly Ending[] = [
  { vowel: false, harmony: 'ı' },
  { vowel: false, harmony: 'i' },
  { vowel: true, harmony: 'i' },
  { vowel: false, harmony: 'ü' },
  { vowel: false, harmony: 'ü' },
  { vowel: false, harmony: 'i' },
  { vowel: true, harmony: 'ı' },
  { vowel: true, harmony: 'i' },
  { vowel: false, harmony: 'i' },
  { vowel: false, harmony: 'u' },
]

// Onlar basamağı 1-9: on, yirmi, otuz, kırk, elli, altmış, yetmiş, seksen, doksan
const TENS: readonly Ending[] = [
  { vowel: false, harmony: 'u' }, // 0: kullanılmaz
  { vowel: false, harmony: 'u' },
  { vowel: true, harmony: 'i' },
  { vowel: false, harmony: 'u' },
  { vowel: false, harmony: 'ı' },
  { vowel: true, harmony: 'i' },
  { vowel: false, harmony: 'ı' },
  { vowel: false, harmony: 'i' },
  { vowel: false, harmony: 'i' },
  { vowel: false, harmony: 'ı' },
]

const SIFIR: Ending = { vowel: false, harmony: 'ı' }
const YUZ: Ending = { vowel: false, harmony: 'ü' }
const BIN: Ending = { vowel: false, harmony: 'i' }
const MILYON: Ending = { vowel: false, harmony: 'u' }
const MILYAR: Ending = { vowel: false, harmony: 'ı' }

/** Sayının okunuşundaki son sözcük (sıfırlar atlanır: 20 → yirmi, 300 → yüz, 4000 → bin). */
function endingOf(n: number): Ending {
  if (n === 0) return SIFIR
  if (n % 10 !== 0) return DIGITS[n % 10] ?? SIFIR
  if (n % 100 !== 0) return TENS[(n % 100) / 10] ?? SIFIR
  if (n % 1000 !== 0) return YUZ
  if (n % 1_000_000 !== 0) return BIN
  if (n % 1_000_000_000 !== 0) return MILYON
  return MILYAR
}

/** Negatif olmayan tam sayının iyelik eki (kesme işareti hariç): 5 → "i", 7 → "si". */
export function possessiveSuffix(n: number): string {
  const { vowel, harmony } = endingOf(Math.abs(Math.trunc(n)))
  return vowel ? `s${harmony}` : harmony
}

const nf = new Intl.NumberFormat('tr-TR')

/** "5'i", "7'si", "1.234'ü": binlik ayraçlı sayı ve iyelik eki. */
export function withPossessive(n: number): string {
  return `${nf.format(n)}'${possessiveSuffix(n)}`
}
