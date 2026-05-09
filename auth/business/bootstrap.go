package business

import (
	"errors"
	"fmt"

	authDb "github.com/alextorq/dns-filter/auth/db"
	"github.com/alextorq/dns-filter/config"
	"gorm.io/gorm"
)

// BootstrapAdmin creates the admin user from env vars on the first run.
// If the admin already exists, the env password is ignored — to recover access,
// delete the user from the DB (or reset via a future explicit reset flow).
func BootstrapAdmin() error {
	cfg := config.GetConfig()
	if cfg.AdminLogin == "" || cfg.AdminPassword == "" {
		return nil
	}

	_, err := authDb.GetUserByLogin(cfg.AdminLogin)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup admin: %w", err)
	}

	hash, err := HashPassword(cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if _, err := authDb.CreateUser(cfg.AdminLogin, hash); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	return nil
}
