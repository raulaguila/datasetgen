package record

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

// shortcut: a record shows its first 200 comments; page them when threads run longer.
const shownComments = 200

// commentRole checks the caller reads the record's collection and returns their role.
func (s *Service) commentRole(ctx context.Context, recordID string) (domain.MemberRole, error) {
	if err := validate.ID(recordID); err != nil {
		return "", err
	}
	of, err := s.Repo.CollectionsOf(ctx, []string{recordID})
	if err != nil {
		return "", err
	}
	notFound := apperror.New(apperror.CodeNotFound, "record not found")
	collectionID, ok := of[recordID]
	if !ok {
		return "", notFound
	}
	role, err := policy.CollectionRole(ctx, s.Collections, collectionID, domain.RoleViewer)
	if apperror.IsCode(err, apperror.CodeNotFound) {
		return "", notFound
	}
	return role, err
}

func (s *Service) Comments(ctx context.Context, recordID string) ([]domain.RecordComment, error) {
	if _, err := s.commentRole(ctx, recordID); err != nil {
		return nil, err
	}
	return s.Repo.Comments(ctx, recordID, shownComments)
}

func (s *Service) AddComment(ctx context.Context, recordID, body string) (*domain.RecordComment, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > 2000 {
		return nil, apperror.New(apperror.CodeInvalidInput, "body must be 1-2000 characters").WithField("body")
	}
	if _, err := s.commentRole(ctx, recordID); err != nil {
		return nil, err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return nil, apperror.New(apperror.CodeForbidden, "an API key cannot comment")
	}
	c := &domain.RecordComment{RecordID: recordID, UserID: reqctx.Actor(ctx), Body: body}
	if err := s.Repo.AddComment(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) DeleteComment(ctx context.Context, recordID, commentID string) error {
	if err := validate.ID(commentID); err != nil {
		return err
	}
	role, err := s.commentRole(ctx, recordID)
	if err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		c, err := uow.Records().Comment(ctx, commentID)
		if err != nil || c.RecordID != recordID {
			return apperror.New(apperror.CodeNotFound, "comment not found")
		}
		if c.UserID != reqctx.Actor(ctx) && !role.Allows(domain.RoleOwner) {
			return apperror.New(apperror.CodeForbidden, "only its author or an owner deletes a comment")
		}
		return uow.Records().DeleteComment(ctx, commentID)
	})
}
