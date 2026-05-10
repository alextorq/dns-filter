package db

import (
	"errors"
	"time"

	database "github.com/alextorq/dns-filter/db"
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

func GetAllClients() ([]Client, error) {
	con := database.GetConnection()
	var clients []Client
	if err := con.Order("id ASC").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

// GetExcludedClients returns rows where DNS filtering is disabled. The store's
// in-memory snapshot is rebuilt from this list — the hot path never reaches
// rows where Filtered=true because they are not "exclusion" facts.
func GetExcludedClients() ([]Client, error) {
	con := database.GetConnection()
	var clients []Client
	if err := con.Where("filtered = ?", false).Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func GetClientByID(id uint) (*Client, error) {
	con := database.GetConnection()
	var c Client
	if err := con.First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func GetClientByIP(ip string) (*Client, error) {
	if ip == "" {
		return nil, ErrNotFound
	}
	con := database.GetConnection()
	var c Client
	if err := con.Where("ip = ?", ip).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func GetClientByMAC(mac string) (*Client, error) {
	if mac == "" {
		return nil, ErrNotFound
	}
	con := database.GetConnection()
	var c Client
	if err := con.Where("mac = ?", mac).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func CreateClient(c *Client) error {
	con := database.GetConnection()
	return con.Create(c).Error
}

// UpdateClientFields persists the user-mutable fields. We avoid GORM's full
// Save because it would also rewrite identifier columns from a possibly stale
// in-memory copy — callers usually only want to flip Filtered or set a Name.
func UpdateClientFields(id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	con := database.GetConnection()
	return con.Model(&Client{}).Where("id = ?", id).Updates(fields).Error
}

func DeleteClient(id uint) error {
	con := database.GetConnection()
	return con.Delete(&Client{}, id).Error
}
