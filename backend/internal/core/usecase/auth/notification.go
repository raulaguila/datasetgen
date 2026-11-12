package auth

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// notify is best effort: a lost notice must not cost the login or refresh it reports on.
func (s *Service) notify(ctx context.Context, n domain.Notification) {
	s.warn("Notification lost", s.UOW.Notifications().Create(ctx, &n))
}

// noticeNewClient warns about a login from a client this account never used.
func (s *Service) noticeNewClient(ctx context.Context, userID string, ip *string) {
	if ip == nil || *ip == "" {
		return
	}
	known, err := s.UOW.Sessions().KnownClient(ctx, userID, *ip)
	if err != nil {
		s.warn("New client check failed", err)
		return
	}
	if !known {
		s.notify(ctx, domain.Notification{
			UserID: userID, Level: domain.NotificationInfo, Kind: domain.NotifyNewLogin, Params: map[string]string{"ip": *ip},
			Title: "New login", Message: "New login from " + *ip + ".",
		})
	}
}
