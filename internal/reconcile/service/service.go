package service

import (
	"math"
	"strings"

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
func Run(a, b []model.Row, opt model.Options) model.Result {
	// 1) Нормализация
	for i := range a {
		a[i].NameNorm = normalize(a[i].Name, opt)
	}
	for i := range b {
		b[i].NameNorm = normalize(b[i].Name, opt)
	}

	// 2) Агрегация дублей
	a = aggregate(a, opt)
	b = aggregate(b, opt)

	// 3) Индекс по B (bySku/byName = map[string][]Row)
	idxB := buildIndexB(b)

	// 4) Учёт использованных строк B
	usedB := make(map[string]bool, len(b))

	rows := make([]model.ResultRow, 0, len(a))
	onlyA := make([]map[string]any, 0)

	for _, ar := range a {
		var (
			matched *model.Row
			method  string
			score   *float64
		)

		// (1) Совпадение по SKU
		if s := strings.TrimSpace(ar.Sku); s != "" {
			if list, ok := idxB.bySku[s]; ok && len(list) > 0 {
				if m := chooseBest(list, ar, usedB); m != nil {
					matched = m
					method = "sku"
				}
			}
		}

		// (2) Точное совпадение нормализованного имени
		if matched == nil && strings.TrimSpace(ar.NameNorm) != "" {
			if list, ok := idxB.byName[ar.NameNorm]; ok && len(list) > 0 {
				if m := chooseBest(list, ar, usedB); m != nil {
					matched = m
					method = "exact"
				}
			}
		}

		// (3) Fuzzy (если разрешён и НЕ strict-after-norm)
		// (3) Fuzzy (если разрешён и НЕ strict-after-norm)
if matched == nil && opt.EnableFuzzy && !opt.StrictAfterNorm && strings.TrimSpace(ar.NameNorm) != "" {
	bestName := ""
	best := -1.0

	nuA := extractNumUnits(ar.NameNorm) // пары "число+единица" из A

	for _, candName := range candidateNames(idxB, ar.NameNorm) {
		// ГАРД: пары "число+единица" должны совпасть
		if !equalNumUnits(nuA, extractNumUnits(candName)) {
			continue
		}
		s := bestSimilarity(ar.NameNorm, candName)
		if s > best {
			best = s
			bestName = candName
		}
	}
	if bestName != "" && best >= opt.Threshold {
		if list, ok := idxB.byName[bestName]; ok && len(list) > 0 {
			if m := chooseBest(list, ar, usedB); m != nil {
				matched = m
				method = "fuzzy"
				val := best
				score = &val
			}
		}
	}
}


		if matched != nil {
			rows = append(rows, model.ResultRow{
				Name:   pick(ar.Name, matched.Name),
				Sku:    pick(ar.Sku, matched.Sku),
				QtyA:   ar.Qty,
				QtyB:   matched.Qty,
				Delta:  ar.Qty - matched.Qty,
				Method: method,
				Score:  score,
			})
			markUsed(usedB, matched)
		} else {
			// нет совпадения → OnlyA
			onlyA = append(onlyA, map[string]any{
				"name": ar.Name,
				"sku":  ar.Sku,
				"qty":  ar.Qty,
			})
		}
	}

	// 5) OnlyB: всё из B, что не помечено как использованное
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

// Выбираем неиспользованного кандидата с минимальным |QtyA-QtyB|
func chooseBest(cands []model.Row, ar model.Row, used map[string]bool) *model.Row {
	var best *model.Row
	bestDist := math.MaxFloat64

	for i := range cands {
		kSku := ""
		if s := strings.TrimSpace(cands[i].Sku); s != "" {
			kSku = "sku:" + s
		}
		kName := "name:" + cands[i].NameNorm

		if (kSku != "" && used[kSku]) || used[kName] {
			continue // уже использован
		}

		d := math.Abs(ar.Qty - cands[i].Qty)
		if d < bestDist {
			best = &cands[i]
			bestDist = d
		}
	}
	return best
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
// сравнение отсортированных мультимножеств "число+единица"
func equalNumUnits(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
