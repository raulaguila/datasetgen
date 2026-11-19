package middleware

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
)

const localCollectionID = "collection.id"

// CollectionPath turns /:owner/:slug into the collection's id for the handlers
// behind it (CollectionID). An old path of a renamed or moved collection still
// resolves. It checks no access: the service does, by id, as before.
func CollectionPath(collections interface {
	Resolve(ctx context.Context, owner, slug string) (string, error)
}) fiber.Handler {
	return func(c fiber.Ctx) error {
		id, err := collections.Resolve(c.Context(), c.Params("owner"), c.Params("slug"))
		if err != nil {
			return mapper.WriteError(c, err)
		}
		c.Locals(localCollectionID, id)
		return c.Next()
	}
}

// CollectionID is the id CollectionPath found.
func CollectionID(c fiber.Ctx) string {
	id, _ := c.Locals(localCollectionID).(string)
	return id
}
