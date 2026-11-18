package content

import (
	"strings"
	"unicode/utf8"
)

// Delta, akış sırasında çözümlenen metin parçasıdır. SSE `delta` olayının
// verisi olarak iletilir (ARCHITECTURE §9). Yalnızca geçici önizlemedir: içerik,
// tamamı Validate'ten geçip kaydedilmeden sürüm olmaz.
type Delta struct {
	// Field, sms_text, email_subject veya email_body.
	Field string `json:"field"`
	// Text, bu alana eklenecek çözümlenmiş metindir (JSON kaçışları açılmıştır).
	Text string `json:"text"`
	// Replace true ise alanın önceki önizlemesi silinir ve Text ile başlanır.
	// Her değerin ilk Delta'sında true'dur; böylece aynı anahtar akışta ikinci
	// kez gelirse (JSON'da son değer geçerlidir) önizleme ona uyar.
	Replace bool `json:"replace,omitempty"`
}

type parserState int

const (
	psStart      parserState = iota // '{' bekleniyor
	psKeyOrEnd                      // anahtar veya '}' bekleniyor
	psKeyReq                        // ',' sonrası: anahtar bekleniyor
	psKey                           // anahtar string'i
	psColon                         // ':' bekleniyor
	psValue                         // değer başlangıcı bekleniyor
	psString                        // string değer
	psSkip                          // string olmayan değer atlanıyor
	psAfterValue                    // ',' veya '}' bekleniyor
	psDone                          // nesne bitti
	psFailed                        // söz dizimi hatası; önizleme durur
)

// deltaParser, modelin yapılandırılmış JSON akışını parçalar halinde okur ve
// bilinen üç alanın metnini çözerek Delta olarak verir. Parça sınırları
// herhangi bir yerde olabilir (kaçış dizisinin ortası, UTF-8 baytlarının
// ortası dahil). Söz dizimi hatasında sessizce durur; nihai karar Validate'in
// işidir. Eşzamanlı kullanıma uygun değildir.
type deltaParser struct {
	state   parserState
	pending []byte // tamamlanmamış UTF-8 baytları

	dec dec
	key strings.Builder

	field       string // geçerli değerin bilinen alanı; bilinmiyorsa ""
	run         strings.Builder
	needReplace bool

	depth     int
	skipInStr bool
	skipEsc   bool

	out []Delta
}

// Feed, bir akış parçasını işler ve bu parçadan çıkan Delta'ları döndürür.
func (p *deltaParser) Feed(chunk string) []Delta {
	p.out = nil
	data := chunk
	if len(p.pending) > 0 {
		data = string(p.pending) + chunk
		p.pending = nil
	}
	for i := 0; i < len(data); {
		if p.state == psDone || p.state == psFailed {
			break
		}
		r, size := utf8.DecodeRuneInString(data[i:])
		if r == utf8.RuneError && size == 1 && !utf8.FullRuneInString(data[i:]) {
			// Çok baytlı karakter parça sınırında bölündü; sonraki parçayı bekle.
			p.pending = []byte(data[i:])
			break
		}
		i += size
		p.process(r)
	}
	p.flush()
	return p.out
}

// Partial, tamamlanmış ama doğrulanamamış bir çıktıdan çözümlenebilen alan
// metinlerini döndürür. Bunlar doğrulanmamıştır; yalnızca insan düzenlemesine
// başlangıç olarak sunulur ve asla sürüm olarak kaydedilmez.
func Partial(raw string) Content {
	var p deltaParser
	var c Content
	for _, d := range p.Feed(raw) {
		var dst *string
		switch d.Field {
		case FieldSMSText:
			dst = &c.SMSText
		case FieldEmailSubject:
			dst = &c.EmailSubject
		case FieldEmailBody:
			dst = &c.EmailBody
		default:
			continue
		}
		if d.Replace {
			*dst = ""
		}
		*dst += d.Text
	}
	return c
}

func (p *deltaParser) flush() {
	if p.field == "" {
		return
	}
	if p.run.Len() == 0 && !p.needReplace {
		return
	}
	p.out = append(p.out, Delta{Field: p.field, Text: p.run.String(), Replace: p.needReplace})
	p.run.Reset()
	p.needReplace = false
}

func (p *deltaParser) fail() { p.state = psFailed }

func isJSONSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

func (p *deltaParser) process(r rune) {
	switch p.state {
	case psStart:
		switch {
		case isJSONSpace(r):
		case r == '{':
			p.state = psKeyOrEnd
		default:
			p.fail()
		}
	case psKeyOrEnd, psKeyReq:
		switch {
		case isJSONSpace(r):
		case r == '"':
			p.state = psKey
			p.dec = dec{}
			p.key.Reset()
		case r == '}' && p.state == psKeyOrEnd:
			p.state = psDone
		default:
			p.fail()
		}
	case psKey:
		done, bad := p.dec.step(r, func(x rune) { p.key.WriteRune(x) })
		switch {
		case bad:
			p.fail()
		case done:
			p.state = psColon
		}
	case psColon:
		switch {
		case isJSONSpace(r):
		case r == ':':
			p.state = psValue
		default:
			p.fail()
		}
	case psValue:
		switch {
		case isJSONSpace(r):
		case r == '"':
			p.field = ""
			if name := p.key.String(); knownField(name) {
				p.field = name
				p.needReplace = true
			}
			p.state = psString
			p.dec = dec{}
		case r == ',' || r == '}' || r == ']':
			p.fail()
		default:
			p.state = psSkip
			p.depth = 0
			p.skipInStr = false
			p.skipEsc = false
			p.skip(r)
		}
	case psString:
		done, bad := p.dec.step(r, func(x rune) {
			if p.field != "" {
				p.run.WriteRune(x)
			}
		})
		switch {
		case bad:
			p.fail()
		case done:
			p.flush()
			p.field = ""
			p.state = psAfterValue
		}
	case psSkip:
		p.skip(r)
	case psAfterValue:
		switch {
		case isJSONSpace(r):
		case r == ',':
			p.state = psKeyReq
		case r == '}':
			p.state = psDone
		default:
			p.fail()
		}
	}
}

// skip, string olmayan (sayı, true, null, dizi, nesne) bir değeri atlar. Geçerli
// JSON olup olmadığına bakmaz; yalnızca değerin nerede bittiğini bulur.
func (p *deltaParser) skip(r rune) {
	if p.skipInStr {
		switch {
		case p.skipEsc:
			p.skipEsc = false
		case r == '\\':
			p.skipEsc = true
		case r == '"':
			p.skipInStr = false
		}
		return
	}
	switch r {
	case '"':
		p.skipInStr = true
	case '{', '[':
		p.depth++
	case '}', ']':
		if p.depth > 0 {
			p.depth--
			return
		}
		if r == '}' {
			p.state = psDone
			return
		}
		p.fail()
	case ',':
		if p.depth == 0 {
			p.state = psKeyReq
		}
	}
}

// dec, tek bir JSON string'inin içeriğini rune rune çözer: \n, \", \uXXXX ve
// vekil çiftleri (surrogate pair) dahil. Geçersiz kaçış ve ham kontrol
// karakteri hatadır.
type dec struct {
	esc     bool
	hexLeft int
	hexVal  rune
	hi      rune // bekleyen yüksek vekil
}

// step bir rune işler. Çözülen karakterleri put ile verir. done string'in
// kapandığını, bad söz dizimi hatasını gösterir.
func (d *dec) step(r rune, put func(rune)) (done, bad bool) {
	if d.hexLeft > 0 {
		v, ok := hexValue(r)
		if !ok {
			return false, true
		}
		d.hexVal = d.hexVal<<4 | v
		d.hexLeft--
		if d.hexLeft > 0 {
			return false, false
		}
		code := d.hexVal
		switch {
		case code >= 0xD800 && code <= 0xDBFF:
			d.flushHi(put)
			d.hi = code
		case code >= 0xDC00 && code <= 0xDFFF:
			if d.hi != 0 {
				put(0x10000 + (d.hi-0xD800)<<10 + (code - 0xDC00))
				d.hi = 0
			} else {
				put(utf8.RuneError)
			}
		default:
			d.flushHi(put)
			put(code)
		}
		return false, false
	}
	if d.esc {
		d.esc = false
		var out rune
		switch r {
		case '"', '\\', '/':
			out = r
		case 'b':
			out = '\b'
		case 'f':
			out = '\f'
		case 'n':
			out = '\n'
		case 'r':
			out = '\r'
		case 't':
			out = '\t'
		case 'u':
			d.hexLeft = 4
			d.hexVal = 0
			return false, false
		default:
			return false, true
		}
		d.flushHi(put)
		put(out)
		return false, false
	}
	switch {
	case r == '\\':
		d.esc = true
	case r == '"':
		d.flushHi(put)
		return true, false
	case r < 0x20:
		return false, true
	default:
		d.flushHi(put)
		put(r)
	}
	return false, false
}

// flushHi, eşi gelmeyen yüksek vekili U+FFFD olarak yazar (encoding/json gibi).
func (d *dec) flushHi(put func(rune)) {
	if d.hi != 0 {
		put(utf8.RuneError)
		d.hi = 0
	}
}

func hexValue(r rune) (rune, bool) {
	switch {
	case r >= '0' && r <= '9':
		return r - '0', true
	case r >= 'a' && r <= 'f':
		return r - 'a' + 10, true
	case r >= 'A' && r <= 'F':
		return r - 'A' + 10, true
	}
	return 0, false
}
