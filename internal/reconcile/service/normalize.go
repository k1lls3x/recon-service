package service

import (
	"regexp"
	"sort"
	"strings"

	"recon-service/internal/reconcile/model"
)

// Латиница→кириллица (визуальные двойники)
var lookalikes = map[rune]rune{
	'A': 'А', 'B': 'В', 'C': 'С', 'E': 'Е', 'H': 'Н', 'K': 'К', 'M': 'М', 'O': 'О', 'P': 'Р', 'T': 'Т', 'X': 'Х', 'Y': 'У',
	'a': 'а', 'c': 'с', 'e': 'е', 'o': 'о', 'p': 'р', 'x': 'х',
}

// Разрешаем буквы/цифры/пробелы + десятичные разделители и проценты

// 0,5 → 0.5
var decComma = regexp.MustCompile(`(\d),(\d)`)

// Единицы измерения (используются и для склейки, и для вырезания отдельных токенов)
const unitWord = `мл|л|кг|г|мг|мм|см|м|шт|%`

// СКЛЕЙКА: "48 мм" → "48мм", "3.2  %" → "3.2%"
// (делаем итеративно на всей строке)
var reAttachNumUnit = regexp.MustCompile(`(?i)\b(\d+(?:[.,]\d+)?)(\s*)(` + unitWord + `)\b`)

// ВЫРЕЗАНИЕ ОТДЕЛЬНЫХ токенов-единиц (если включен StripUnits)
// (склеенные пары "48мм" не затрагиваются)
var reUnitTokens = regexp.MustCompile(`(?i)\b(` + unitWord + `)\b`)

// Поиск склеенных пар для гарда fuzzy: ["48мм", "66м", "3.2%"]
var reNumUnitFind = regexp.MustCompile(`(?i)\d+(?:\.\d+)?(?:` + unitWord + `)`)

// === normalize — главный конвейер ===
func normalize(s string, opt model.Options) string {
	if s == "" {
		return ""
	}
	out := s

	// 1) Унификация символов: ё→е, лат↔кир, спец-разделители → пробел
	if opt.Unify {
		out = unifyLookalikes(out)
	}

	// 2) Регистр
	if opt.Lowercase {
		out = strings.ToLower(out)
	}

	// 3) Десятичные: 3,2 → 3.2 (делаем ДО чистки пунктуации)
	out = decComma.ReplaceAllString(out, "$1.$2")

	// 4) Очищаем пунктуацию, но сохраняем . , %
	if opt.Normalization {
		out = removePunctToSpaces(out)
	} else {
		out = collapseSpaces(out)
	}

	// 5) СКЛЕЙКА "число + единица": "48 мм"→"48мм", "3.2 %"→"3.2%"
	out = attachNumberUnitsEverywhere(out)

	// 6) Опционально удаляем ОТДЕЛЬНЫЕ единицы (склеенные не трогаем)
	if opt.StripUnits {
		out = stripUnitTokens(out)
	}

	// 7) Сортировка токенов (после склейки пары остаются единым токеном)
	if opt.TokenSort {
		out = tokenSort(out)
	}

	return strings.TrimSpace(out)
}

// ===== helpers =====

// Ё→Е, лат↔кир по lookalikes, ×/*/x/· → пробел
func unifyLookalikes(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case 'ё':
			r = 'е'
		case 'Ё':
			r = 'Е'
		case '×', '*', 'x', 'X', '·':
			r = ' '
		default:
			if rr, ok := lookalikes[r]; ok {
				r = rr
			}
		}
		b = append(b, r)
	}
	return string(b)
}

var punct = regexp.MustCompile(`[^\p{L}\p{N}\s.,%]+`) // разрешаем . , %
func removePunctToSpaces(s string) string {
  return collapseSpaces(punct.ReplaceAllString(s, " "))
}
// Итеративная СКЛЕЙКА "число + единица" по всей строке
func attachNumberUnitsEverywhere(s string) string {
	prev := ""
	out := collapseSpaces(s)
	for out != prev {
		prev = out
		out = reAttachNumUnit.ReplaceAllString(out, "$1$3")
		out = collapseSpaces(out)
	}
	return out
}

// Убираем ЕДИНИЦЫ, если они стоят ОТДЕЛЬНЫМИ токенами
// (склеенные пары типа "48мм" остаются)
func stripUnitTokens(s string) string {
	return collapseSpaces(reUnitTokens.ReplaceAllString(s, " "))
}

// Лексикографическая сортировка токенов
func tokenSort(s string) string {
	f := strings.Fields(s)
	sort.Strings(f)
	return strings.Join(f, " ")
}

// Схлопывание пробелов
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Мультимножество склеенных "число+единица" для гарда fuzzy (используй в service.go)
func extractNumUnits(s string) []string {
	mm := reNumUnitFind.FindAllString(s, -1)
	sort.Strings(mm)
	return mm
}
