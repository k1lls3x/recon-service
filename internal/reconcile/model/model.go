package model

type Mapping struct {
	NameKey   string // имя колонки с наименованием
	QtyKey    string // имя колонки с количеством
	SkuKey    string // имя колонки с артикулом (опционально)
	UseSku    bool   // использовать ли артикул
	HeaderRow int    // строка заголовков (1-based)
}

type Options struct {
	Normalization   bool    // убрать пунктуацию, схлопнуть пробелы и т.п.
	TokenSort       bool    // сортировка токенов (нож туристический == туристический нож)
	StripUnits      bool    // срезать единицы измерения в конце
	Unify           bool    // латиница→кириллица (двойники A/А, P/Р и т.п.)
	Lowercase       bool    // привести к нижнему регистру
	EnableFuzzy     bool    // включить нечеткое сопоставление, если нет точного
	Threshold       float64 // порог схожести для fuzzy (0..1)
	StrictAfterNorm bool    // только точные совпадения после нормализации (без fuzzy)
}

type Row struct {
	Name     string  // исходное наименование
	Sku      string  // артикул
	Qty      float64 // количество
	NameNorm string  // нормализованное имя (считается для «таблицы B»)
}

type ResultRow struct {
	Name   string   `json:"name"`
	Sku    string   `json:"sku"`
	QtyA   float64  `json:"qtyA"`
	QtyB   float64  `json:"qtyB"`
	Delta  float64  `json:"delta"`
	Method string   `json:"method"`           // sku | exact | fuzzy
	Score  *float64 `json:"score,omitempty"`  // метрика схожести для fuzzy
}


type Result struct {
    Rows  []ResultRow      `json:"rows"`
    OnlyA []map[string]any `json:"onlyA"`
    OnlyB []map[string]any `json:"onlyB"`
    Opts  Options          `json:"opts"`
    MapA  Mapping          `json:"mapA"`
    MapB  Mapping          `json:"mapB"`
}
