// Package account holds what a logged in user does with their own account.
package account

import (
	"context"
	"net/http"
	"regexp"
	"slices"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

const sessionsCap = 100

var languageFormat = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// Service implements input.AccountService.
type Service struct {
	UOW   output.AccountStore
	Perms output.PermissionReader
	Clock output.Clock
	Files output.FileStore
}

var _ input.AccountService = (*Service)(nil)

func New(uow output.AccountStore, perms output.PermissionReader, clock output.Clock, files output.FileStore) *Service {
	return &Service{UOW: uow, Perms: perms, Clock: clock, Files: files}
}

// Profile loads permissions live, so a client refreshing it sees grants at once.
func (s *Service) Profile(ctx context.Context, userID string) (*domain.Profile, error) {
	user, err := s.UOW.Users().FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	eff, err := s.Perms.LoadEffective(ctx, userID)
	if err != nil {
		return nil, err
	}
	prefs, err := s.Preferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := s.UOW.Roles().ForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	avatarAt, err := s.UOW.Users().AvatarAt(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile := &domain.Profile{User: *user, Preferences: *prefs, Roles: []string{}, AvatarAt: avatarAt}
	for _, r := range roles {
		if r.Enabled {
			profile.Roles = append(profile.Roles, r.Name)
		}
	}
	if eff != nil {
		profile.IsSuperuser = eff.IsSuperuser
		profile.Permissions = eff.ByResource
	}
	return profile, nil
}

func (s *Service) Rename(ctx context.Context, userID, name string) (*domain.Profile, error) {
	name, err := policy.ValidateName(name)
	if err != nil {
		return nil, err
	}
	if err := s.UOW.Users().UpdateName(ctx, userID, name); err != nil {
		return nil, err
	}
	return s.Profile(ctx, userID)
}

// avatarTypes are the pictures a browser shows; SVG stays out, since it can carry script.
var avatarTypes = []string{"image/png", "image/jpeg", "image/webp"}

func avatarKey(userID string) string { return "avatars/" + userID }

// SetAvatar trusts the bytes, not a declared type: the type is sniffed from the content.
func (s *Service) SetAvatar(ctx context.Context, userID string, image []byte) error {
	if len(image) == 0 || len(image) > domain.AvatarMaxBytes {
		return apperror.New(apperror.CodeInvalidInput, "picture must be at most 100 KB").WithField("avatar")
	}
	if !slices.Contains(avatarTypes, http.DetectContentType(image)) {
		return apperror.New(apperror.CodeInvalidInput, "picture must be PNG, JPEG or WebP").WithField("avatar")
	}
	if err := s.Files.Put(ctx, avatarKey(userID), image); err != nil {
		return err
	}
	now := s.Clock.Now()
	return s.UOW.Users().SetAvatarAt(ctx, userID, &now)
}

// RemoveAvatar deletes the file first: if the second step fails, the profile
// points at a missing picture, never at one the user asked to remove.
func (s *Service) RemoveAvatar(ctx context.Context, userID string) error {
	if err := s.Files.Delete(ctx, avatarKey(userID)); err != nil {
		return err
	}
	return s.UOW.Users().SetAvatarAt(ctx, userID, nil)
}

func (s *Service) Avatar(ctx context.Context, userID string) ([]byte, string, error) {
	image, err := s.Files.Get(ctx, avatarKey(userID))
	if err != nil {
		return nil, "", err
	}
	return image, http.DetectContentType(image), nil
}

func (s *Service) Sessions(ctx context.Context, userID string) ([]domain.Session, error) {
	return s.UOW.Sessions().ListActive(ctx, userID, s.Clock.Now(), sessionsCap)
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	if err := validate.ID(sessionID); err != nil {
		return err
	}
	revoked, err := s.UOW.Sessions().RevokeOwned(ctx, sessionID, userID, s.Clock.Now(), domain.RevokeUserLogout)
	if err != nil {
		return err
	}
	if !revoked {
		return apperror.New(apperror.CodeNotFound, "session not found")
	}
	return nil
}

func (s *Service) RevokeOtherSessions(ctx context.Context, userID, currentSessionID string) error {
	return s.UOW.Sessions().RevokeOthers(ctx, userID, currentSessionID, s.Clock.Now(), domain.RevokeUserLogout)
}

func (s *Service) Preferences(ctx context.Context, userID string) (*domain.Preferences, error) {
	prefs, err := s.UOW.Preferences().Find(ctx, userID)
	if apperror.IsNotFound(err) {
		return &domain.Preferences{UserID: userID, Theme: domain.ThemeSystem, Language: domain.DefaultLanguage}, nil
	}
	return prefs, err
}

func (s *Service) UpdatePreferences(ctx context.Context, p domain.Preferences) (*domain.Preferences, error) {
	switch p.Theme {
	case domain.ThemeSystem, domain.ThemeLight, domain.ThemeDark:
	default:
		return nil, apperror.New(apperror.CodeInvalidInput, "theme must be system, light or dark").WithField("theme")
	}
	if !languageFormat.MatchString(p.Language) {
		return nil, apperror.New(apperror.CodeInvalidInput, "language must look like pt-BR").WithField("language")
	}
	if err := s.UOW.Preferences().Save(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) Notifications(ctx context.Context, userID string, page domain.Page) ([]domain.Notification, int64, error) {
	return s.UOW.Notifications().List(ctx, userID, page)
}

func (s *Service) CountUnread(ctx context.Context, userID string) (int64, error) {
	return s.UOW.Notifications().CountUnread(ctx, userID)
}

// MarkRead takes nil for "all". An empty but present list is a mistake, not "all".
func (s *Service) MarkRead(ctx context.Context, userID string, ids []string) error {
	if ids != nil {
		if len(ids) == 0 || len(ids) > 100 {
			return apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
		}
		ids = slices.Compact(slices.Sorted(slices.Values(ids))) // a repeated id would fail the count check
		for _, id := range ids {
			if err := validate.ID(id); err != nil {
				return err
			}
		}
	}
	found, err := s.UOW.Notifications().MarkRead(ctx, userID, ids, s.Clock.Now())
	if err != nil {
		return err
	}
	if !found {
		return apperror.New(apperror.CodeNotFound, "notification not found")
	}
	return nil
}
