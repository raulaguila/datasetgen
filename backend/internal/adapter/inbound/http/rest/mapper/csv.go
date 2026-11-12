package mapper

import "strings"

// CSVCell keeps a spreadsheet from running user text as a formula: names,
// emails and record values come from users, and "=HYPERLINK(...)" would
// otherwise be live.
func CSVCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
