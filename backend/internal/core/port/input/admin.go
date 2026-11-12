package input

import "context"

// BootstrapCommand describes the first admin.
type BootstrapCommand struct {
	Name     string
	Email    string
	Password string
}

type AdminService interface {
	Bootstrap(context.Context, BootstrapCommand) (bool, error)
	Promote(context.Context, string) error
	Disable(context.Context, string) error
	Enable(context.Context, string) error
}
