package auth

import (
	"context"
	"errors"

	"berkut-scc/config"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"github.com/gofrs/uuid/v5"
)

type SessionManager struct {
	store  store.SessionStore
	cfg    *config.AppConfig
	logger *utils.Logger
}

func NewSessionManager(store store.SessionStore, cfg *config.AppConfig, logger *utils.Logger) *SessionManager {
	return &SessionManager{store: store, cfg: cfg, logger: logger}
}

func (m *SessionManager) Create(ctx context.Context, user *store.User, roles []string, ip, userAgent string) (*Session, error) {
	id := uuid.Must(uuid.NewV4()).String()
	var csrf string
	var err error
	if m.cfg.CSRFKey != "" {
		csrf, err = GenerateCSRF(m.cfg.CSRFKey, id)
	} else {
		csrf, err = utils.RandString(32)
	}
	if err != nil {
		return nil, err
	}
	now := utils.NowUTC()
	sessionTTL := m.cfg.EffectiveSessionTTL()
	sess := &Session{
		ID:         id,
		UserID:     user.ID,
		Username:   user.Username,
		Roles:      roles,
		IP:         ip,
		UserAgent:  userAgent,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(sessionTTL),
		CSRFToken:  csrf,
	}
	if err := m.store.SaveSession(ctx, &store.SessionRecord{
		ID:         sess.ID,
		UserID:     sess.UserID,
		Username:   sess.Username,
		Roles:      sess.Roles,
		IP:         sess.IP,
		UserAgent:  sess.UserAgent,
		CSRFToken:  sess.CSRFToken,
		CreatedAt:  sess.CreatedAt,
		LastSeenAt: sess.LastSeenAt,
		ExpiresAt:  sess.ExpiresAt,
	}); err != nil {
		return nil, err
	}
	return sess, nil
}

func (m *SessionManager) Refresh(ctx context.Context, sessID string) error {
	return m.store.UpdateActivity(ctx, sessID, utils.NowUTC(), m.cfg.EffectiveSessionTTL())
}

func (m *SessionManager) Rotate(ctx context.Context, sessID string) (*Session, error) {
	old, err := m.store.GetSession(ctx, sessID)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, errors.New("session not found")
	}
	_ = m.store.DeleteSession(ctx, sessID, old.Username)
	return m.Create(ctx, &store.User{ID: old.UserID, Username: old.Username}, old.Roles, old.IP, old.UserAgent)
}

func (m *SessionManager) Delete(ctx context.Context, sessID string) error {
	return m.store.DeleteSession(ctx, sessID, "")
}
