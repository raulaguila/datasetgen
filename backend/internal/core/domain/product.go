package domain

import "time"

// ProductFilter selects one page of products.
type ProductFilter struct {
	Page    Page
	Cursor  bool // created_at DESC, id DESC; omits the expensive total count
	After   *ProductCursor
	Search  string
	Enabled *bool // nil lists both
	Sort    string
	Order   string
}

// Product is the example app domain entity (app.product).
type Product struct {
	ID        string
	Name      string
	Price     string // decimal as string to avoid float
	Enabled   bool
	Version   int // bumped by every update; see ProductPatch.Version
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProductPatch holds the fields of an update; nil keeps the stored value.
type ProductPatch struct {
	Name    *string
	Price   *string
	Enabled *bool
	// Version, when set, is the version the caller read: the update is refused
	// with a conflict if the row changed since.
	Version *int
}

type ProductCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}
