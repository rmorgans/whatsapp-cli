package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// timeNow is the clock function used for relative timestamp formatting.
// Tests override this to produce deterministic output.
var timeNow = time.Now

const (
	maxColWidth   = 40
	maxTotalWidth = 120
	colSeparator  = "  "
	truncSuffix   = "\u2026" // ellipsis
)

// GenericFormat renders an Envelope as human-readable text.
// It inspects the JSON shape of Data to pick the right presentation.
func GenericFormat(env Envelope) (string, error) {
	// Rule 1: error envelope
	if !env.Success {
		msg := "unknown error"
		if env.Error != nil {
			msg = *env.Error
		}
		return "Error: " + msg, nil
	}

	// Rule 2: null or empty data
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return "No results.", nil
	}

	// Decode into generic interface{} to inspect shape.
	var raw interface{}
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		return "", fmt.Errorf("generic format: unmarshal data: %w", err)
	}

	switch v := raw.(type) {
	case []interface{}:
		return formatArray(v)
	case map[string]interface{}:
		return formatKeyValue(v), nil
	default:
		// Scalar: string, float64, bool
		return formatScalar(v), nil
	}
}

// formatArray dispatches between empty, array-of-objects, and array-of-scalars.
func formatArray(arr []interface{}) (string, error) {
	if len(arr) == 0 {
		return "No results.", nil
	}

	// Check if first element is an object — if so, treat all as objects.
	if _, ok := arr[0].(map[string]interface{}); ok {
		return formatTable(arr), nil
	}

	// Array of scalars: one per line.
	var b strings.Builder
	for i, item := range arr {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatScalar(item))
	}
	return b.String(), nil
}

// formatTable renders an array of objects as a columnar table.
func formatTable(arr []interface{}) string {
	// Collect all keys across all objects for deterministic column order.
	keySet := map[string]struct{}{}
	rows := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, obj)
		for k := range obj {
			keySet[k] = struct{}{}
		}
	}

	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build header names and cell strings.
	headers := make([]string, len(keys))
	for i, k := range keys {
		headers[i] = formatHeader(k)
	}

	cellGrid := make([][]string, len(rows))
	for r, obj := range rows {
		cellGrid[r] = make([]string, len(keys))
		for c, k := range keys {
			cellGrid[r][c] = formatCell(obj[k])
		}
	}

	// Compute column widths (capped at maxColWidth).
	widths := make([]int, len(keys))
	for c, h := range headers {
		widths[c] = runeWidth(h)
	}
	for _, row := range cellGrid {
		for c, cell := range row {
			w := runeWidth(cell)
			if w > widths[c] {
				widths[c] = w
			}
		}
	}
	for c := range widths {
		if widths[c] > maxColWidth {
			widths[c] = maxColWidth
		}
	}

	// Determine how many columns fit within maxTotalWidth.
	visibleCols := 0
	usedWidth := 0
	for c, w := range widths {
		needed := w
		if c > 0 {
			needed += len(colSeparator)
		}
		if usedWidth+needed > maxTotalWidth {
			break
		}
		usedWidth += needed
		visibleCols++
	}
	if visibleCols == 0 && len(widths) > 0 {
		visibleCols = 1 // always show at least one column
	}

	// Render.
	var b strings.Builder

	// Header line.
	for c := 0; c < visibleCols; c++ {
		if c > 0 {
			b.WriteString(colSeparator)
		}
		b.WriteString(padOrTruncate(headers[c], widths[c]))
	}
	b.WriteString("\n")

	// Data rows.
	for _, row := range cellGrid {
		for c := 0; c < visibleCols; c++ {
			if c > 0 {
				b.WriteString(colSeparator)
			}
			b.WriteString(padOrTruncate(row[c], widths[c]))
		}
		b.WriteString("\n")
	}

	// Strip trailing whitespace from each line.
	return trimTrailingWhitespace(b.String())
}

// formatKeyValue renders a single object as aligned key: value pairs.
func formatKeyValue(obj map[string]interface{}) string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Compute display names and max width for alignment.
	names := make([]string, len(keys))
	maxNameLen := 0
	for i, k := range keys {
		names[i] = formatKeyName(k)
		if n := runeWidth(names[i]); n > maxNameLen {
			maxNameLen = n
		}
	}

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		name := names[i]
		val := formatCell(obj[k])
		// Right-pad name so colons + values align.
		padding := maxNameLen - runeWidth(name)
		b.WriteString(name)
		b.WriteString(":")
		b.WriteString(strings.Repeat(" ", padding+1))
		b.WriteString(" ")
		b.WriteString(val)
	}
	return trimTrailingWhitespace(b.String())
}

// formatHeader converts a JSON key to an uppercase header.
// Underscores become spaces.
func formatHeader(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, "_", " "))
}

// formatKeyName converts a JSON key to a title-cased display name.
// Underscores become spaces.
func formatKeyName(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// formatCell converts a single value to its display string.
func formatCell(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case bool:
		if val {
			return "yes"
		}
		return "no"
	case string:
		if ts, ok := tryParseTimestamp(val); ok {
			return formatTimestamp(ts)
		}
		return val
	case float64:
		// Print integers without decimal point.
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case map[string]interface{}:
		return "{...}"
	case []interface{}:
		return "[...]"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// formatScalar formats a top-level scalar value.
// Strings are returned as-is (no timestamp conversion) since bare
// timestamp scalars are not a meaningful command output shape.
func formatScalar(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return formatCell(v)
}

// tryParseTimestamp attempts to parse a string as RFC3339.
func tryParseTimestamp(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// formatTimestamp renders a timestamp as relative or short date.
func formatTimestamp(t time.Time) string {
	now := timeNow()
	diff := now.Sub(t)

	if diff < 0 {
		// Future timestamps get short date treatment.
		return shortDate(t, now)
	}

	if diff < 7*24*time.Hour {
		return relativeTime(diff, now, t)
	}
	return shortDate(t, now)
}

func relativeTime(diff time.Duration, now, t time.Time) string {
	// "yesterday" if the calendar date is one day before now's date.
	nowDate := now.In(t.Location())
	if isYesterday(nowDate, t) {
		return "yesterday"
	}

	days := int(diff.Hours() / 24)
	if days >= 2 {
		return fmt.Sprintf("%d days ago", days)
	}

	hours := int(diff.Hours())
	if hours >= 1 {
		return fmt.Sprintf("%dh ago", hours)
	}

	mins := int(diff.Minutes())
	if mins >= 1 {
		return fmt.Sprintf("%dm ago", mins)
	}

	return "just now"
}

func isYesterday(now, t time.Time) bool {
	loc := t.Location()
	ny, nm, nd := now.In(loc).Date()
	ty, tm, td := t.Date()
	nowDay := time.Date(ny, nm, nd, 0, 0, 0, 0, loc)
	tDay := time.Date(ty, tm, td, 0, 0, 0, 0, loc)
	return nowDay.Sub(tDay) == 24*time.Hour
}

func shortDate(t time.Time, now time.Time) string {
	if t.Year() == now.Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2 2006")
}

// padOrTruncate pads a string to width or truncates with ellipsis.
func padOrTruncate(s string, width int) string {
	w := runeWidth(s)
	if w <= width {
		return s + strings.Repeat(" ", width-w)
	}
	// Truncate to width-1 runes then add ellipsis.
	return truncateRunes(s, width-1) + truncSuffix
}

// truncateRunes returns the first n runes of s.
func truncateRunes(s string, n int) string {
	i := 0
	for count := 0; count < n && i < len(s); count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

// runeWidth returns the number of runes in s.
func runeWidth(s string) int {
	return utf8.RuneCountInString(s)
}

// trimTrailingWhitespace removes trailing spaces from each line
// and the final trailing newline.
func trimTrailingWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	// Remove trailing empty lines.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
