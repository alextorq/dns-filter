package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Client is the canonical record for a known DNS client.
//
// One of IP / MAC / Token is the canonical identifier in a given deployment:
//   - LAN mode populates IP today and (after the PR3 ARP-watcher) MAC.
//   - Public mode populates Token, leaves IP/MAC empty.
//
// Filtered=true means DNS filtering is applied to this client; Filtered=false
// excludes them from the bloom/blocklist check on the hot path. The default is
// true so that newly registered clients inherit the global behavior — explicit
// exclusion is the rarer, more deliberate state.
type Client struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	IP    string `json:"ip" gorm:"index"`
	MAC   string `json:"mac" gorm:"index"`
	Token string `json:"token" gorm:"index"`

	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Vendor   string `json:"vendor"`

	Filtered bool `json:"filtered" gorm:"default:true"`

	LastSeen *time.Time `json:"last_seen,omitempty"`
}

// ErrNotFound is returned by lookups when no client matches.
var ErrNotFound = errors.New("client not found")

// Repo is the clients persistence adapter. Construct it once in the
// composition root and pass it to the clients module and background workers.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo {
	return &Repo{db: conn}
}

func (r *Repo) GetAll() ([]Client, error) {
	var clients []Client
	if err := r.db.Order("id ASC").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

// GetExcluded returns rows where DNS filtering is disabled. The store's
// in-memory snapshot is rebuilt from this list — the hot path never reaches
// rows where Filtered=true because they are not "exclusion" facts.
func (r *Repo) GetExcluded() ([]Client, error) {
	var clients []Client
	if err := r.db.Where("filtered = ?", false).Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func (r *Repo) GetByID(id uint) (*Client, error) {
	var c Client
	if err := r.db.First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repo) GetByIP(ip string) (*Client, error) {
	if ip == "" {
		return nil, ErrNotFound
	}
	var c Client
	if err := r.db.Where("ip = ?", ip).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repo) GetByMAC(mac string) (*Client, error) {
	if mac == "" {
		return nil, ErrNotFound
	}
	var c Client
	if err := r.db.Where("mac = ?", mac).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repo) Create(c *Client) error {
	filtered := c.Filtered
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		// GORM applies gorm:"default:true" to a false zero value on Create.
		// Write the caller's explicit exclusion state back in the same
		// transaction so DB and the hot-path snapshot cannot diverge.
		if !filtered {
			if err := tx.Model(c).UpdateColumn("filtered", false).Error; err != nil {
				return err
			}
			c.Filtered = false
		}
		return nil
	})
}

// UpdateFields persists the user-mutable fields. We avoid GORM's full
// Save because it would also rewrite identifier columns from a possibly stale
// in-memory copy — callers usually only want to flip Filtered or set a Name.
func (r *Repo) UpdateFields(id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&Client{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repo) Delete(id uint) error {
	return r.db.Delete(&Client{}, id).Error
}
