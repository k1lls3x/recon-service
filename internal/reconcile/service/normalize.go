package service

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"strconv"
)

// --- базовая нормализация текста ---

var spaceCleaner = strings.NewReplacer(
	"\u00A0", " ", // NBSP
	"\u202F", " ", // NNBSP (узкий NBSP)
	"\u2007", " ", // FIGURE SPACE
	"\t", " ",
	"\r\n", "\n",
	"\r", "\n",
)

func toLowerRu(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

// --- извлечение и унификация размеров ---

// 1200x800, 1200х800, 1200×800, 1200*800, + необязательная третья грань и суффикс мм/mm
var reDim = regexp.MustCompile(`(?i)\b(\d{2,5})\s*[xх×\*]\s*(\d{2,5})(?:\s*[xх×\*]\s*(\d{1,5}))?\s*(?:мм|mm)?\b`)

// normalizeDims возвращает нормализованный токен размера (пример: "1200x800")
// Если находит 3-мерный размер, возвращает первые две грани — этого обычно достаточно для паллетов.
func normalizeDims(s string) (dim string, out string) {
	out = s
	m := reDim.FindStringSubmatch(s)
	if len(m) >= 3 {
		a := strings.TrimLeft(m[1], "0")
		b := strings.TrimLeft(m[2], "0")
		if a == "" { a = "0" }
		if b == "" { b = "0" }
		dim = a + "x" + b
		// вырезаем исходный фрагмент размера из строки, чтобы не мешал токенизации
		out = strings.Replace(s, m[0], " ", 1)
	}
	return dim, strings.TrimSpace(out)
}

// --- синонимы/канонизация товарных слов ---

// единичные замены токенов
var tokenSynonyms = map[string]string{
	// базовые синонимы для контекста паллет/поддонов
	"паллет":   "поддон",
	"палета":   "поддон",
	"паллета":  "поддон",
	"паллетта": "поддон",
	"палет":    "поддон",
	// ед. изм. и мусор — выкинем позже как стоп-слова, но на всякий случай канонизируем
	"шт.": "шт",
	"л.":  "л",
}

// выражения «евро поддон» в любых формах → один токен "европоддон"
var reEuroPoddon1 = regexp.MustCompile(`\bевро\s*[-\s]*поддон\b`)
var reEuroPoddon2 = regexp.MustCompile(`\bподдон\s*[-\s]*евро\b`)
var reEuroPoddon3 = regexp.MustCompile(`\bевроподдон\b`)

// стоп-слова, не влияющие на сущность
var stop = map[string]struct{}{
	"мм": {}, "mm": {}, "шт": {}, "уп": {}, "упак": {}, "ед": {}, "изм": {},
}

// --- токенизация с учётом кириллицы и цифр ---

func splitTokens(s string) []string {
	sb := strings.Builder{}
	tokens := []string{}
	flush := func() {
		if sb.Len() == 0 {
			return
		}
		t := sb.String()
		sb.Reset()
		tokens = append(tokens, t)
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			// кириллица/латиница/цифры — ок
			sb.WriteRune(r)
		case r == 'x': // оставим 'x' как часть размеров, но тут таких уже нет (мы их вырезали ранее)
			sb.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// --- публичная функция нормализации названия ---

// NameKey строит порядконезависимый нормализованный ключ номенклатуры.
// Примеры (все дадут один ключ):
//  - "Поддон Евро 1200х800мм"
//  - "Европоддон 1200×800"
//  - "евро-поддон 1200*800 мм"
//  => "1200x800 европоддон"
func NameKey(raw string) string {
	if raw == "" {
		return ""
	}

	// 1) базовая чистка
	s := spaceCleaner.Replace(raw)
	s = toLowerRu(s)
	// унификация знака размера: кириллическая 'х', умножение '×' → латинская 'x'
	s = strings.NewReplacer("х", "x", "×", "x", "X", "x").Replace(s)
	s = strings.TrimSpace(s)

	// 2) вытащим размер
	dim, rest := normalizeDims(s)

	// 3) Канонизируем "европоддон"
	rest = reEuroPoddon1.ReplaceAllString(rest, "европоддон")
	rest = reEuroPoddon2.ReplaceAllString(rest, "европоддон")
	// Если слово склеено, оставим как есть (редко встречается с дефисами/без пробела)
	rest = strings.ReplaceAll(rest, "евро-поддон", "европоддон")

	// 4) Токенизация и синонимы
	rawTokens := splitTokens(rest)
	tokens := make([]string, 0, len(rawTokens)+1)

	seen := map[string]struct{}{}
	add := func(t string) {
		if t == "" {
			return
		}
		if _, bad := stop[t]; bad {
			return
		}
		if _, ok := seen[t]; ok {
			return
		}
		seen[t] = struct{}{}
		tokens = append(tokens, t)
	}

	for _, t := range rawTokens {
		if rep, ok := tokenSynonyms[t]; ok {
			t = rep
		}
		// склеенные варианты "европоддон" уже превращены выше, проверим ещё раз
		if reEuroPoddon3.MatchString(t) {
			t = "европоддон"
		}
		// выкинем чисто цифровые хвосты, которые уже «ушли» в размер
		if _, err := strconv.Atoi(t); err == nil {
			continue
		}
		add(t)
	}

	// 5) Добавим размер отдельным токеном (если был)
	if dim != "" {
		add(dim)
	}

	if len(tokens) == 0 {
		return ""
	}

	// 6) Порядконезависимый ключ
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}
