package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"regexp"
	"github.com/rs/zerolog"

	"recon-service/internal/config"
	"recon-service/internal/fileio"
	"recon-service/internal/reconcile/model"
	recSvc "recon-service/internal/reconcile/service"
)

// Reconcile возвращает http.HandlerFunc, чтобы вы могли вызвать его как
// r.Post("/reconcile", recHnd.Reconcile(cfg, logger)) в роутере.
func Reconcile(cfg config.Config, logger zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Привяжем req_id из заголовка, если middleware его проставил
		reqID := r.Header.Get("X-Request-ID")
		log := logger
		if reqID != "" {
			log = logger.With().Str("req_id", reqID).Logger()
		}

		defer r.Body.Close()
		if err := r.ParseMultipartForm(200 << 20); err != nil { // 200MB
			http.Error(w, "bad multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}

		fileA, headerA, err := r.FormFile("fileA")
		if err != nil {
			http.Error(w, "missing fileA: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer fileA.Close()

		fileB, headerB, err := r.FormFile("fileB")
		if err != nil {
			http.Error(w, "missing fileB: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer fileB.Close()

		// Читаем таблицы (auto-encoding CSV, XLS/XLSX и т.д. внутри fileio)
		rowsA, err := fileio.ReadAnyMaps(fileA, headerA.Filename, atoi(r.FormValue("a_header_row"), 1))
		if err != nil {
			http.Error(w, "failed to read A: "+err.Error(), http.StatusBadRequest)
			return
		}
		rowsB, err := fileio.ReadAnyMaps(fileB, headerB.Filename, atoi(r.FormValue("b_header_row"), 1))
		if err != nil {
			http.Error(w, "failed to read B: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Маппинги
		ma := model.Mapping{
			NameKey:   r.FormValue("a_name"),
			QtyKey:    r.FormValue("a_qty"),
			SkuKey:    r.FormValue("a_sku"),
			UseSku:    toBool(r.FormValue("a_use_sku"), true), // осознанный дефолт
			HeaderRow: atoi(r.FormValue("a_header_row"), 1),
		}
		mb := model.Mapping{
			NameKey:   r.FormValue("b_name"),
			QtyKey:    r.FormValue("b_qty"),
			SkuKey:    r.FormValue("b_sku"),
			UseSku:    toBool(r.FormValue("b_use_sku"), true), // осознанный дефолт
			HeaderRow: atoi(r.FormValue("b_header_row"), 1),
		}

		// Опции (дефолты чекбоксов = false)
		opt := model.Options{
			Normalization:   toBool(r.FormValue("normalization"), false),
			TokenSort:       toBool(r.FormValue("token_sort"), false),
			StripUnits:      toBool(r.FormValue("strip_units"), false),
			Unify:           toBool(r.FormValue("unify"), false),
			Lowercase:       toBool(r.FormValue("lowercase"), false),
			EnableFuzzy:     toBool(r.FormValue("enable_fuzzy"), false),
			StrictAfterNorm: toBool(r.FormValue("strict_after_norm"), false),
			Threshold:       toFloat(r.FormValue("threshold"), 0.85),
		}

		// В модельные строки + фильтр шапок
		aRows := toRowsFiltered(rowsA, ma)
		bRows := toRowsFiltered(rowsB, mb)

		// Запуск сверки
		res := recSvc.Run(aRows, bRows, opt)

		// Эхо того, что реально применилось (для отладки в UI и curl)
		res.Opts = opt
		res.MapA = ma
		res.MapB = mb

		// Ответ
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			log.Error().Err(err).Msg("write json")
			return
		}

		log.Info().
			Int("rowsA", len(aRows)).
			Int("rowsB", len(bRows)).
			Dur("elapsed", time.Since(start)).
			Msg("reconcile done")
	}
}

func toRowsFiltered(maps []map[string]string, m model.Mapping) []model.Row {
	rows := make([]model.Row, 0, len(maps))
	for _, rec := range maps {
		// эвристика — пропустить строки, похожие на шапку
		if looksLikeHeaderMap(rec) {
			continue
		}
		name := strings.TrimSpace(rec[m.NameKey])
		if name == "" {
			continue
		}
		qty := toNumber(rec[m.QtyKey])
		sku := ""
		if m.UseSku && m.SkuKey != "" {
			sku = strings.TrimSpace(rec[m.SkuKey])
		}
		rows = append(rows, model.Row{Name: name, Sku: sku, Qty: qty})
	}
	return rows
}

func looksLikeHeaderMap(m map[string]string) bool {
    cnt := 0
    for _, v := range m {
        s := strings.ToLower(strings.TrimSpace(v))
        if strings.Contains(s, "наимен") || strings.Contains(s, "артикул") ||
           strings.Contains(s, "колич") || strings.Contains(s, "итого") {
            cnt++
        }
    }
    return cnt >= 2
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return i
}

func toBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func toFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return def
	}
	return f
}

func toNumber(s string) float64 {
    s = strings.TrimSpace(s)
    // вычистим спец-пробелы
    s = strings.Map(func(r rune) rune {
        switch r {
        case '\u00A0', '\u2009', '\u202F': // NBSP, thin space, narrow NBSP
            return -1
        default:
            return r
        }
    }, s)
    neg := false
    if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
        neg = true
        s = s[1:len(s)-1]
    }
    s = strings.ReplaceAll(s, " ", "")
    s = strings.ReplaceAll(s, ",", ".")
    // убираем всё, что не цифра/точка/минус
    s = regexp.MustCompile(`[^0-9.\-]`).ReplaceAllString(s, "")
    v, err := strconv.ParseFloat(s, 64)
    if err != nil { return 0 }
    if neg { v = -v }
    return v
}
