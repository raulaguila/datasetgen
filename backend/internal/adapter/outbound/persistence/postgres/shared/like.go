package shared

import "strings"

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Contains builds the ILIKE pattern matching s literally, anywhere in the text.
func Contains(s string) string {
	return "%" + likeEscaper.Replace(s) + "%"
}
