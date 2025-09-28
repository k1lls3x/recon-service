package service

import (
	"math"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"recon-service/internal/reconcile/model"
)

// aggregate duplicates by key (prefer SKU; otherwise normalized name)
func aggregate(rows []model.Row, opt model.Options) []model.Row {
	agg := make(map[string]model.Row)
	for _, r := range rows {
		if r.NameNorm == "" {
			r.NameNorm = normalize(r.Name, opt)
		}
		key := r.Sku
		if key == "" {
			key = r.NameNorm
		}
		if ex, ok := agg[key]; ok {
			ex.Qty += r.Qty
			agg[key] = ex
		} else {
			agg[key] = r
		}
	}
	out := make([]model.Row, 0, len(agg))
	for _, v := range agg {
		out = append(out, v)
	}
	return out
}

// Run — основная сверка. Строит индекс по B, матчит A→B, считает дельты.
// Параллельная реализация: обрабатываем строки A в пуле воркеров, а
// критические секции (выбор кандидата с учётом usedB) защищаем мьютексом.
func Run(a, b []model.Row, opt model.Options) model.Result {
	// 1) Нормализация
	for i := range a {
		if a[i].NameNorm == "" {
			a[i].NameNorm = normalize(a[i].Name, opt)
		}
	}
	for i := range b {
		if b[i].NameNorm == "" {
			b[i].NameNorm = normalize(b[i].Name, opt)
		}
	}

	// 2) Агрегация дублей (SKU → иначе NameNorm)
	a = aggregate(a, opt)
	b = aggregate(b, opt)

	// 3) Индекс по B
	idxB := buildIndexB(b)

	// 4) Учёт использованных записей B (ключи: "sku:<sku>", "name:<norm>")
	usedB := make(map[string]bool, len(b))
	var usedMu sync.Mutex

	type out struct {
		row   *model.ResultRow
		onlyA map[string]any
	}
	outs := make([]out, len(a))

	// Кэши нормализаций для кандидатов (безопасны для гонок)
	var normCache sync.Map  // candName -> candNorm
	var unitsCache sync.Map // candNorm -> []string

	// Воркер на одну строку A (по индексу i)
	worker := func(i int) {
		ar := a[i]
		var matched *model.Row
		var method string
		var score *float64

		// (1) Совпадение по SKU
		if s := strings.TrimSpace(ar.Sku); s != "" {
			usedMu.Lock()
			if list, ok := idxB.bySku[s]; ok && len(list) > 0 {
				if m := chooseBest(list, ar, usedB); m != nil {
					matched = m
					method = "sku"
					markUsed(usedB, matched)
				}
			}
			usedMu.Unlock()
		}

		// (2) Точное совпадение нормализованного имени
		if matched == nil && strings.TrimSpace(ar.NameNorm) != "" {
			usedMu.Lock()
			if list, ok := idxB.byName[ar.NameNorm]; ok && len(list) > 0 {
				if m := chooseBest(list, ar, usedB); m != nil {
					matched = m
					method = "exact"
					markUsed(usedB, matched)
				}
			}
			usedMu.Unlock()
		}

		// (3) Fuzzy (тяжёлую часть — оценку схожести — считаем вне мьютекса)
		if matched == nil && opt.EnableFuzzy && !opt.StrictAfterNorm && strings.TrimSpace(ar.NameNorm) != "" {
			nuA := extractNumUnits(ar.NameNorm)

			bestName := ""
			best := -1.0

			// 3.1 Кандидаты из инверт-индекса
			for _, candName := range idxB.candidateNames(ar.NameNorm) {
				// normalize(candidate) — с кэшем
				var candNorm string
				if v, ok := normCache.Load(candName); ok {
					candNorm = v.(string)
				} else {
					candNorm = normalize(candName, opt)
					normCache.Store(candName, candNorm)
				}
				// guard по единицам — с кэшем
				var nuB []string
				if v, ok := unitsCache.Load(candNorm); ok {
					nuB = v.([]string)
				} else {
					nuB = extractNumUnits(candNorm)
					unitsCache.Store(candNorm, nuB)
				}
				if !equalNumUnitsSoft(nuA, nuB) {
					continue
				}

				s := bestSimilarity(ar.NameNorm, candNorm)
				if s > opt.Threshold && s > best {
					best = s
					bestName = candName
				}
			}

			// 3.2 Fallback — полный проход по byName, если индекс пуст
			if bestName == "" {
				for candName := range idxB.byName {
					var candNorm string
					if v, ok := normCache.Load(candName); ok {
						candNorm = v.(string)
					} else {
						candNorm = normalize(candName, opt)
						normCache.Store(candName, candNorm)
					}
					var nuB []string
					if v, ok := unitsCache.Load(candNorm); ok {
						nuB = v.([]string)
					} else {
						nuB = extractNumUnits(candNorm)
						unitsCache.Store(candNorm, nuB)
					}
					if !equalNumUnitsSoft(nuA, nuB) {
						continue
					}
					s := bestSimilarity(ar.NameNorm, candNorm)
					if s > opt.Threshold && s > best {
						best = s
						bestName = candName
					}
				}
			}

			// 3.3 Фиксация результата
			if bestName != "" {
				usedMu.Lock()
				if list, ok := idxB.byName[bestName]; ok && len(list) > 0 {
					if m := chooseBest(list, ar, usedB); m != nil {
						matched = m
						method = "fuzzy"
						score = &best
						markUsed(usedB, matched)
					}
				}
				usedMu.Unlock()
			}
		}

		// 4) Запись результата, сохраняем порядок
		if matched != nil {
			outs[i].row = &model.ResultRow{
				Name:   pick(ar.Name, matched.Name),
				Sku:    pick(ar.Sku, matched.Sku),
				QtyA:   ar.Qty,
				QtyB:   matched.Qty,
				Delta:  ar.Qty - matched.Qty,
				Method: method,
				Score:  score,
			}
		} else {
			outs[i].onlyA = map[string]any{
				"name": ar.Name,
				"sku":  ar.Sku,
				"qty":  ar.Qty,
			}
		}
	}

	// Пул воркеров по числу CPU
	nw := runtime.NumCPU()
	if nw < 1 {
		nw = 1
	}

	type job struct{ i int }
	jobs := make(chan job, nw*2)
	var wg sync.WaitGroup

	for w := 0; w < nw; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				worker(j.i)
			}
		}()
	}

	for i := range a {
		jobs <- job{i: i}
	}
	close(jobs)
	wg.Wait()

	// Сборка результатов в стабильном порядке
	rows := make([]model.ResultRow, 0, len(a))
	onlyA := make([]map[string]any, 0)
	for i := 0; i < len(a); i++ {
		if outs[i].row != nil {
			rows = append(rows, *outs[i].row)
		}
		if outs[i].onlyA != nil {
			onlyA = append(onlyA, outs[i].onlyA)
		}
	}

	// 5) OnlyB: всё из B, что не использовано
	onlyB := make([]map[string]any, 0, len(b))
	for _, br := range b {
		usedBySku := false
		usedByName := false
		if s := strings.TrimSpace(br.Sku); s != "" {
			usedBySku = usedB["sku:"+s]
		}
		if n := strings.TrimSpace(br.NameNorm); n != "" {
			usedByName = usedB["name:"+n]
		}
		if !(usedBySku || usedByName) {
			onlyB = append(onlyB, map[string]any{
				"name": br.Name,
				"sku":  br.Sku,
				"qty":  br.Qty,
			})
		}
	}

	return model.Result{
		Rows:  rows,
		OnlyA: onlyA,
		OnlyB: onlyB,
	}
}

func pick(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// Выбираем неиспользованного кандидата по smart-правилам:
// 1) similarity desc
// 2) при близком similarity (<= 0.02) — ненулевой qtyB лучше нулевого
// 3) затем минимальная |QtyA-QtyB|
// 4) стабильная ничья по индексу
func chooseBest(cands []model.Row, ar model.Row, used map[string]bool) *model.Row {
	bestIdx := -1
	bestSim := -1.0
	bestNonZero := false
	bestDelta := math.MaxFloat64

	for i := range cands {
		// пропускаем уже использованных
		kSku := ""
		if s := strings.TrimSpace(cands[i].Sku); s != "" {
			kSku = "sku:" + s
		}
		kName := "name:" + cands[i].NameNorm
		if (kSku != "" && used[kSku]) || used[kName] {
			continue
		}

		sim := bestSimilarity(ar.NameNorm, cands[i].NameNorm)
		nonZero := cands[i].Qty != 0
		delta := math.Abs(ar.Qty - cands[i].Qty)

		better := false
		// 1) similarity
		if sim > bestSim+0.02 {
			better = true
		} else if math.Abs(sim-bestSim) <= 0.02 {
			// 2) ненулевой qtyB предпочтительнее
			if nonZero != bestNonZero {
				better = nonZero && !bestNonZero
			} else if delta < bestDelta {
				// 3) меньшая |Δ|
				better = true
			} else if delta == bestDelta && i < bestIdx {
				// 4) стабильность
				better = true
			}
		}
		if better {
			bestIdx = i
			bestSim = sim
			bestNonZero = nonZero
			bestDelta = delta
		}
	}
	if bestIdx == -1 {
		return nil
	}
	return &cands[bestIdx]
}

func markUsed(used map[string]bool, r *model.Row) {
	if r == nil {
		return
	}
	if s := strings.TrimSpace(r.Sku); s != "" {
		used["sku:"+s] = true
	}
	if n := strings.TrimSpace(r.NameNorm); n != "" {
		used["name:"+n] = true
	}
}

// --------- ВСПОМОГАТЕЛЬНОЕ: нормализация и «число+единица» ---------

// normalize — совместимая обёртка вокруг твоего NameKey (из normalize.go)
func normalize(s string, _ model.Options) string {
	return NameKey(s) // см. normalize.go
}

// extractNumUnits вытаскивает из нормализованной строки размер "1200x800"
// и шаблоны вида "<число><единица>" (л, кг, г, мл, мм, см, м, pcs...).
var (
	reDimToken   = regexp.MustCompile(`^\d{2,5}x\d{2,5}$`)
	reNumUnit    = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)(шт|л|кг|г|гр|мл|мм|см|м|pcs|pc|l|kg|g|ml|mm|cm|m)$`)
	unitCanonMap = map[string]string{
		"шт": "pcs", "pcs": "pcs", "pc": "pcs",
		"л": "l", "l": "l",
		"кг": "kg", "kg": "kg",
		"г": "g", "гр": "g", "g": "g",
		"мл": "ml", "ml": "ml",
		"мм": "mm", "mm": "mm",
		"см": "cm", "cm": "cm",
		"м": "m", "m": "m",
	}
)

func extractNumUnits(norm string) []string {
	if norm == "" {
		return nil
	}
	toks := strings.Fields(norm)
	out := make([]string, 0, 4)
	for _, t := range toks {
		if reDimToken.MatchString(t) {
			out = append(out, t)
			continue
		}
		if m := reNumUnit.FindStringSubmatch(t); len(m) == 3 {
			num := strings.ReplaceAll(m[1], ",", ".")
			unit := unitCanonMap[m[2]]
			if unit != "" {
				out = append(out, num+unit)
			}
		}
	}
	return out
}

// сравнение отсортированных мультимножеств "число+единица"
func equalNumUnitsSoft(a, b []string) bool {
	// канонизируем десятичный разделитель
	for i := range a {
		a[i] = strings.ReplaceAll(a[i], ",", ".")
	}
	for i := range b {
		b[i] = strings.ReplaceAll(b[i], ",", ".")
	}

	ac, bc := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	if len(ac) > len(bc)+1 {
		return false
	}
	i, j, miss := 0, 0, 0
	for i < len(ac) && j < len(bc) {
		if ac[i] == bc[j] {
			i++
			j++
			continue
		}
		miss++
		if miss > 1 {
			return false
		}
		if ac[i] < bc[j] {
			i++
		} else {
			j++
		}
	}
	miss += (len(ac) - i) + (len(bc) - j)
	return miss <= 1
}
