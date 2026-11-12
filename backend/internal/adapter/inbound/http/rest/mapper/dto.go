package mapper

import "github.com/raulaguila/dataset/internal/core/domain"

// PaginationDTO is the pagination block of every paged list.
type PaginationDTO struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
}

// Pagination describes the page answered out of total rows.
func Pagination(page domain.Page, total int64) PaginationDTO {
	limit := int64(page.Limit)
	return PaginationDTO{
		Page:       page.Number,
		Limit:      page.Limit,
		TotalItems: total,
		TotalPages: (total + limit - 1) / limit,
	}
}

// PreferencesDTO is shared by the user object and the preferences routes.
type PreferencesDTO struct {
	Theme    string `json:"theme" example:"system"`
	Language string `json:"language" example:"pt-BR"`
}

// Preferences converts the domain value.
func Preferences(p domain.Preferences) PreferencesDTO {
	return PreferencesDTO{Theme: p.Theme, Language: p.Language}
}

// Permissions lists the action names per resource. "*" already grants every
// action on every resource, so when it is present the other entries are left out.
func Permissions(p map[string]domain.Action) map[string][]string {
	if all, ok := p[domain.ResourceAll]; ok {
		return map[string][]string{domain.ResourceAll: domain.ActionNames(all)}
	}
	out := make(map[string][]string, len(p))
	for resource, actions := range p {
		out[resource] = domain.ActionNames(actions)
	}
	return out
}
