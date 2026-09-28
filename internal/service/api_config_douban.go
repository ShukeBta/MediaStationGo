package service

import (
	"context"
	"strings"
)

// MigrateDoubanCookie moves the legacy runtime cookie once. An administrator
// can subsequently clear the encrypted API credential without it reappearing.
func (s *APIConfigService) MigrateDoubanCookie(ctx context.Context, cookie string) error {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return nil
	}
	const key = "internal.douban_cookie_migrated"
	done, err := s.repo.Setting.Get(ctx, key)
	if err != nil {
		return err
	}
	if done == "1" {
		return nil
	}
	row, err := s.findByProvider(ctx, "douban")
	if err != nil {
		return err
	}
	if row != nil && row.APIKey == "" && strings.TrimSpace(cookie) != "" {
		if _, err := s.Update(ctx, "douban", APIConfigPatch{APIKey: &cookie}); err != nil {
			return err
		}
	}
	return s.repo.Setting.Set(ctx, key, "1")
}
