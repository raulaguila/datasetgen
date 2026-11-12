package shared

// orderColumns is the allowlist behind ORDER BY, which takes no bind parameters.
var orderColumns = map[string]bool{"name": true, "email": true, "status": true, "created_at": true}

// OrderBy sorts the table alias by column, then by id so pages stay stable on
// ties. An unknown column falls back to name.
func OrderBy(alias, column, order string) string {
	if !orderColumns[column] {
		column = "name"
	}
	direction := " ASC"
	if order == "desc" {
		direction = " DESC"
	}
	return alias + "." + column + direction + ", " + alias + ".id"
}
