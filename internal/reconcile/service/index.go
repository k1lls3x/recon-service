package service

import (
	"math"
	"sort"

	"recon-service/internal/reconcile/model"
)

// индекс для быстрого поиска по B
type indexB struct {
    bySku  map[string][]model.Row
    byName map[string][]model.Row
    inv    map[string]map[string]struct{} // trigram -> set(normalized name)
}


func buildIndexB(rows []model.Row) indexB {
    idx := indexB{
        bySku:  make(map[string][]model.Row),
        byName: make(map[string][]model.Row),
        inv:    make(map[string]map[string]struct{}),
    }
    for _, r := range rows {
        if r.Sku != "" {
            idx.bySku[r.Sku] = append(idx.bySku[r.Sku], r)
        }
        if r.NameNorm != "" {
            nn := r.NameNorm
            idx.byName[nn] = append(idx.byName[nn], r)
            for _, g := range trigrams(nn) {
                if idx.inv[g] == nil {
                    idx.inv[g] = make(map[string]struct{})
                }
                idx.inv[g][nn] = struct{}{}
            }
        }
    }
    return idx
}

func trigrams(s string) []string {
	rs := []rune(s)
	if len(rs) < 3 {
		return []string{string(rs)}
	}
	out := make([]string, 0, len(rs)-2)
	for i := 0; i+2 < len(rs); i++ {
		out = append(out, string(rs[i:i+3]))
	}
	return out
}

func trigramSet(s string) map[string]struct{} {
	m := make(map[string]struct{})
	if s == "" {
		return m
	}
	// pad with spaces
	p := " " + s + " "
	runes := []rune(p)
	if len(runes) < 3 {
		m[p] = struct{}{}
		return m
	}
	for i := 0; i <= len(runes)-3; i++ {
		m[string(runes[i:i+3])] = struct{}{}
	}
	return m
}

func candidateNames(idx indexB, norm string) []string {
	seen := make(map[string]int)
	for tg := range trigramSet(norm) {
		for name := range idx.inv[tg] {
			seen[name]++
		}
	}
	// convert to slice; sort by hits desc for determinism
	type kv struct{ name string; hits int }
	arr := make([]kv, 0, len(seen))
	for n, h := range seen {
		arr = append(arr, kv{n, h})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].hits != arr[j].hits {
			return arr[i].hits > arr[j].hits
		}
		return arr[i].name < arr[j].name
	})
	out := make([]string, 0, len(arr))
	for _, kv := range arr {
		out = append(out, kv.name)
	}
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
