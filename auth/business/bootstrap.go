package business

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// BootstrapAdmin creates the admin user from env vars on the first run.
// If the admin already exists, the env password is ignored — to recover access,
// delete the user from the DB (or reset via a future explicit reset flow).
func (m *Module) BootstrapAdmin() error {
	if m.adminLogin == "" || m.adminPassword == "" {
		return nil
	}

	_, err := m.repo.GetUserByLogin(m.adminLogin)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup admin: %w", err)
	}

	hash, err := HashPassword(m.adminPassword)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if _, err := m.repo.CreateUser(m.adminLogin, hash); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	return nil
}
