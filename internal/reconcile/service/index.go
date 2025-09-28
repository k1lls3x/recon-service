package service

import (
	"math"
	"sort"
	"strings"

	"recon-service/internal/reconcile/model"
)

// индекс для быстрого поиска по B
type Index struct {
	bySku  map[string][]model.Row
	byName map[string][]model.Row
	inv    map[string]map[string]struct{} // trigram -> set(normalized name)
}

func buildIndexB(rows []model.Row) *Index {
	idx := &Index{
		bySku:  make(map[string][]model.Row),
		byName: make(map[string][]model.Row),
		inv:    make(map[string]map[string]struct{}),
	}

	for _, r := range rows {
		if r.Sku != "" {
			idx.bySku[r.Sku] = append(idx.bySku[r.Sku], r)
		}
		if r.NameNorm == "" {
			continue
		}
		nn := r.NameNorm
		idx.byName[nn] = append(idx.byName[nn], r)

		for g := range trigramSet(nn) {
			bucket, ok := idx.inv[g]
			if !ok {
				bucket = make(map[string]struct{})
				idx.inv[g] = bucket
			}
			bucket[nn] = struct{}{}
		}
	}

	return idx
}

// service.go
func trigramSet(s string) map[string]struct{} {
	m := make(map[string]struct{})
	if s == "" {
		return m
	}
	p := " " + s + " "
	r := []rune(p)
	if len(r) < 3 {
		m[p] = struct{}{}
		return m
	}
	for i := 0; i <= len(r)-3; i++ {
		m[string(r[i:i+3])] = struct{}{}
	}
	return m
}

func (idx *Index) candidateNames(norm string) []string {
	if norm == "" {
		return nil
	}
	seen := make(map[string]struct{})
	for g := range trigramSet(norm) {
		if bucket, ok := idx.inv[g]; ok {
			for nn := range bucket {
				seen[nn] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for nn := range seen {
		out = append(out, nn)
	}
	sort.Strings(out) // для детерминированного порядка
	return out
}

func similarity(a, b string) float64 {
	// normalized Damerau-Levenshtein similarity in [0..1]
	if a == "" && b == "" {
		return 1
	}
	if a == "" || b == "" {
		return 0
	}
	d := damerauLevenshtein(a, b)
	m := len([]rune(a))
	if mb := len([]rune(b)); mb > m {
		m = mb
	}
	if m == 0 {
		return 1
	}
	return 1 - float64(d)/float64(m)
}

// tokenSort: сортируем токены по алфавиту (устойчиво к порядку слов)
func tokenSort(s string) string {
	if s == "" {
		return s
	}
	t := strings.Fields(s)
	sort.Strings(t)
	return strings.Join(t, " ")
}

func tokenSortSimilarity(a, b string) float64 {
	sa := tokenSort(a)
	sb := tokenSort(b)
	return similarity(sa, sb)
}

func bestSimilarity(a, b string) float64 {
	x := similarity(a, b)
	y := tokenSortSimilarity(a, b)
	if y > x {
		return y
	}
	return x
}

// silence unused import warning (when damerauLevenshtein comes from another file)
var _ = math.Min
