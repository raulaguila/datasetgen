package domain

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
	maxPageNumber    = 1_000_000 // keeps OFFSET far from overflow
)

// Page is one window of a list.
type Page struct {
	Number int
	Limit  int
}

// NewPage brings client values into range.
func NewPage(number, limit int) Page {
	if limit < 1 {
		limit = defaultPageLimit
	}
	return Page{Number: max(1, min(number, maxPageNumber)), Limit: min(limit, maxPageLimit)}
}

// Offset is the number of rows before this page.
func (p Page) Offset() int { return (p.Number - 1) * p.Limit }
