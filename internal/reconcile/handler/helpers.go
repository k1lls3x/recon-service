// нормализуем имя колонки: нижний регистр, убираем служ.символы/множественные пробелы/ё→е
package handler
import (

	"strings"

	"regexp"

	"recon-service/internal/reconcile/model"

)

func normHeaderKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("\u00A0", " ", "\u202F", " ", "ё", "е").Replace(s) // NBSP/NNBSP
	s = regexp.MustCompile(`[^\p{L}\p{N}]+`).ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// ищем реальный ключ в записи по желаемому имени.
// Поддерживает варианты через "|" (например: "Наименование|Номенклатура")
func resolveKey(rec map[string]string, want string) string {
	want = strings.TrimSpace(want)
	if want == "" {
		return ""
	}
	// 1) поддержка альтернатив: "a|b|c"
	alts := strings.Split(want, "|")
	for i := range alts {
		alts[i] = strings.TrimSpace(alts[i])
	}

	// 2) точное совпадение (как есть)
	for _, a := range alts {
		if _, ok := rec[a]; ok {
			return a
		}
	}

	// 3) нормализованные сравнения и contains (для составных заголовков)
	//    пример: "сальдо на конец периода количество" содержит "количество"
	nWant := normHeaderKey(alts[0]) // первый как основной
	var nWantAll []string
	for _, a := range alts {
		nWantAll = append(nWantAll, normHeaderKey(a))
	}

	// сначала пройдём по всем ключам и найдём лучшее совпадение
	bestKey := ""
	bestScore := 0
	for k := range rec {
		nk := normHeaderKey(k)
		// точное по нормализованному
		for _, n := range nWantAll {
			if nk == n {
				return k
			}
		}
		// частичное: want ⊂ key  или  key ⊂ want
		score := 0
		for _, n := range nWantAll {
			if strings.Contains(nk, n) || strings.Contains(n, nk) {
				score = max(score, len(n))
			}
		}
		// специальные эвристики
		if strings.Contains(nWant, "колич") && strings.Contains(nk, "колич") {
			score += 100
		}
		if strings.Contains(nWant, "наимен") && strings.Contains(nk, "наимен") {
			score += 100
		}
		if score > bestScore {
			bestScore, bestKey = score, k
		}
	}
	return bestKey
}

func max(a, b int) int { if a > b { return a }; return b }

func toRowsFiltered(maps []map[string]string, m model.Mapping) []model.Row {
	rows := make([]model.Row, 0, len(maps))
	for _, rec := range maps {
		// пропуск явных шапок
		if looksLikeHeaderMap(rec) {
			continue
		}

		nameKey := resolveKey(rec, m.NameKey)
		qtyKey  := resolveKey(rec, m.QtyKey)
		skuKey  := resolveKey(rec, m.SkuKey)

		name := strings.TrimSpace(rec[nameKey])
		if name == "" {
			continue
		}

		qty := toNumber(rec[qtyKey])

		sku := ""
		if m.UseSku && skuKey != "" {
			sku = strings.TrimSpace(rec[skuKey])
		}

		// отсечь полностью пустые строки
		if name == "" && sku == "" && qty == 0 {
			continue
		}
		rows = append(rows, model.Row{Name: name, Sku: sku, Qty: qty})
	}
	return rows
}
