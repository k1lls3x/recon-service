package fileio

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// ---------- НОРМАЛИЗАЦИЯ ТЕКСТА (общая для xls/xlsx/csv) ----------

var spcCleaner = strings.NewReplacer(
	"\u00A0", " ", // NBSP
	"\u202F", " ", // NNBSP (узкий NBSP)
	"\u2007", " ", // FIGURE SPACE
	"\r\n", "\n",
	"\r", "\n",
	"\t", " ",
)

// NBSP/NNBSP/FIGURE SPACE → обычные пробелы; типографский минус → '-', схлопываем пробелы
func normalizeCell(s string) string {
	if s == "" {
		return s
	}
	s = spcCleaner.Replace(s)
	s = strings.ReplaceAll(s, "−", "-") // U+2212 -> '-'
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// аккуратная склейка верх/низ шапки: "Верх / Низ", без дублей
func joinHeaderParts(top, bottom string) string {
	top = normalizeCell(top)
	bottom = normalizeCell(bottom)
	switch {
	case top == "" && bottom == "":
		return ""
	case top == "":
		return bottom
	case bottom == "":
		return top
	default:
		lt, lb := strings.ToLower(top), strings.ToLower(bottom)
		if lt == lb || strings.Contains(lb, lt) {
			return bottom
		}
		if strings.Contains(lt, lb) {
			return top
		}
		return normalizeCell(top + " / " + bottom)
	}
}

// ---------- HEURISTICS: второй ярус шапки или данные? ----------

var headerKeywords = []string{
	"ед", "изм",        // ед. изм.
	"колич", "остат",   // количество/остаток
	"начальн", "приход", "расход", "конечн",
	"артикул", "номенклатур", "склад", "организац",
}

// грубо: «числовая» ли ячейка
func isNumericish(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r := strings.NewReplacer("\u00A0", "", "\u202F", "", " ", "", ",", ".", "−", "-")
	s = r.Replace(s)
	sb := strings.Builder{}
	for _, ch := range s {
		if unicode.IsDigit(ch) || ch == '.' || ch == '-' {
			sb.WriteRune(ch)
		}
	}
	if sb.Len() == 0 {
		return false
	}
	_, err := strconv.ParseFloat(sb.String(), 64)
	return err == nil
}

// строка похожа на «второй ярус шапки», если в ней >=2 «служебных» слов и словесных ячеек не меньше, чем числовых
func looksLikeSecondHeaderRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	nonempty, words, nums := 0, 0, 0
	hit := 0
	for _, c := range cells {
		t := strings.ToLower(normalizeCell(c))
		if t == "" {
			continue
		}
		nonempty++
		for _, kw := range headerKeywords {
			if strings.Contains(t, kw) {
				hit++
				break
			}
		}
		hasLetter := false
		for _, r := range t {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
		if hasLetter && !isNumericish(t) {
			words++
		} else if isNumericish(t) {
			nums++
		}
	}
	if nonempty == 0 {
		return false
	}
	return hit >= 2 && words >= nums
}

// ---------- ОБЩИЕ ХЕЛПЕРЫ ДЛЯ ВСЕХ ПАРСЕРОВ ----------

// pickHeader — берёт шапку с headerRow (1-based).
// Если следующая строка выглядит как «второй ярус шапки», склеивает верх+низ по столбцам.
func pickHeader(rows [][]string, headerRow int) []string {
	idx := headerRow - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
		if idx < 0 {
			idx = 0
		}
	}

	var top, bot []string
	if idx >= 0 && idx < len(rows) {
		top = rows[idx]
	}
	useBot := idx+1 < len(rows) && looksLikeSecondHeaderRow(rows[idx+1])
	if useBot {
		bot = rows[idx+1]
	}

	maxCols := 0
	if len(top) > maxCols {
		maxCols = len(top)
	}
	if len(bot) > maxCols {
		maxCols = len(bot)
	}
	if maxCols == 0 {
		maxCols = 1
	}

	out := make([]string, maxCols)
	for i := 0; i < maxCols; i++ {
		var t, b string
		if i < len(top) {
			t = top[i]
		}
		if i < len(bot) {
			b = bot[i]
		}
		h := joinHeaderParts(t, b)
		if h == "" {
			h = fmt.Sprintf("Column %d", i+1)
		}
		out[i] = h
	}
	return out
}

// --------- РАСПОЗНАВАНИЕ КОЛОНОК КОЛИЧЕСТВА / ОСТАТКА ---------

// канонизация заголовка для матчинга
func canonHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	// убираем пробелы, дефисы, точки, слэши, кавычки, скобки
	repl := strings.NewReplacer(" ", "", "-", "", "—", "", "_", "", ".", "", ",", "", "/", "", "\\", "", "(", "", ")", "", "\"", "", "'", "")
	s = repl.Replace(s)
	return s
}

// isQtyHeader — многоязычный и «разговорный» детектор: qty/quantity/stock/on hand/balance/remain и русские варианты
func isQtyHeader(h string) bool {
	k := canonHeader(h)
	if k == "" {
		return false
	}
	// явные совпадения
	explicit := []string{
		"qty", "quantity", "qtty", "onhand", "onhands", "stock", "balance", "remaining", "remain", "remains",
		"kolvo", "kolichestvo", "kolich", "kolichestvo", "ostatok", "ost", "ostatkona", "ostatnaconets",
		"konetsostatok", "konechnyjostatok", "konechnyiostatok", "itog", "vsego",
	}
	for _, e := range explicit {
		if k == e {
			return true
		}
	}
	// сочетания/фразы
	if strings.Contains(k, "qty") || strings.Contains(k, "quantity") || strings.Contains(k, "onhand") || strings.Contains(k, "stock") {
		return true
	}
	if strings.Contains(k, "remain") || strings.Contains(k, "balance") {
		return true
	}
	if strings.Contains(k, "ostat") { // остаток / ост / остатокнаконе(ц) / кон. остаток
		return true
	}
	if strings.Contains(k, "kolich") || strings.Contains(k, "kolvo") {
		return true
	}
	if strings.Contains(k, "konechn") && strings.Contains(k, "ostat") {
		return true
	}
	// часто встречается "Остатокнаконец", "Остнаконец", "Остаток(кон)"
	if strings.Contains(k, "nakonec") || (strings.Contains(k, "kon") && strings.Contains(k, "ost")) {
		return true
	}
	return false
}

// ParseRuFloat — робастный парсер RU/EN чисел: NBSP/узкие пробелы, запятая/точка, скобки-минус, смешанные разделители
func ParseRuFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// унификация минуса и пробелов
	s = strings.ReplaceAll(s, "−", "-")     // U+2212 -> '-'
	s = strings.ReplaceAll(s, "\u00A0", "") // NBSP
	s = strings.ReplaceAll(s, "\u202F", "") // NNBSP
	s = strings.ReplaceAll(s, "\u2007", "") // FIGURE SPACE
	s = strings.ReplaceAll(s, " ", "")      // обычный пробел

	neg := false
	// скобочная отрицательность: (123,45)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = s[1 : len(s)-1]
	}

	// Если есть и запятая, и точка: считаем запятые разделителями тысяч → удаляем
	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	} else if strings.Count(s, ",") >= 1 && !strings.Contains(s, ".") {
		// если только запятые — последнюю запятую считаем десятичной
		if strings.Count(s, ",") > 1 {
			last := strings.LastIndex(s, ",")
			s = strings.ReplaceAll(s[:last], ",", "")
			s = s[:last] + "." + s[last+1:]
		} else {
			s = strings.ReplaceAll(s, ",", ".")
		}
	}

	// фильтр на мусор: оставим цифры, точку, минус
	sb := strings.Builder{}
	for _, r := range s {
		if unicode.IsDigit(r) || r == '.' || r == '-' {
			sb.WriteRune(r)
		}
	}
	s = sb.String()
	if s == "" || s == "-" || s == "." || s == "-." {
		return 0, false
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		f = -f
	}
	return f, true
}

// ---------- КОНВЕРТАЦИЯ ТАБЛИЦЫ В MAPS ----------

// rowsToMaps — конвертирует AoA в []map по заголовкам.
// Если строка под шапкой распознана как «второй ярус», начинаем с headerRow+1.
// Дополнительно: для колонок с количеством/остатком нормализуем число через ParseRuFloat.
func rowsToMaps(rows [][]string, headers []string, headerRow int) []map[string]string {
	idx := headerRow - 1
	useBot := idx+1 < len(rows) && looksLikeSecondHeaderRow(rows[idx+1])

	// первая строка данных
	start := headerRow
	if useBot {
		start = headerRow + 1
	}
	if start < 0 {
		start = 0
	}
	if start > len(rows) {
		start = len(rows)
	}

	// заранее посчитаем, какие колонки — количественные
	qtyCol := make(map[int]bool, len(headers))
	for i, h := range headers {
		if isQtyHeader(h) {
			qtyCol[i] = true
		}
	}

	var out []map[string]string
	for r := start; r < len(rows); r++ {
		rec := rows[r]
		if len(rec) == 0 {
			continue
		}
		m := make(map[string]string, len(headers))
		empty := true

		for c := 0; c < len(headers); c++ {
			var v string
			if c < len(rec) {
				v = normalizeCell(rec[c])
			}

			if qtyCol[c] {
				if f, ok := ParseRuFloat(v); ok {
					v = strconv.FormatFloat(f, 'f', -1, 64) // "495558.073" / "118" / "-5"
				}
			}

			if strings.TrimSpace(v) != "" {
				empty = false
			}
			m[headers[c]] = v
		}

		if !empty {
			out = append(out, m)
		}
	}
	return out
}

// ReadAnyMaps — выбирает парсер по расширению и возвращает []map[header]value.
func ReadAnyMaps(r io.Reader, filename string, headerRow int) ([]map[string]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return readXLSX(r, headerRow)
	case ".xls":
		return readXLS(r, headerRow)
	case ".csv":
		return readCSV(r, headerRow)
	default:
		return nil, fmt.Errorf("unsupported file: %s", filename)
	}
}

// (на всякий случай)
func itoa(i int) string { return strconv.Itoa(i) }
