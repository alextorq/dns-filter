package sync

import (
	"fmt"

	easy_list "github.com/alextorq/dns-filter/source/business/use-cases/sync/easy-list"
	"github.com/alextorq/dns-filter/source/db"
)

// NewDefaultLoaders builds the feature-owned production mapping from source
// identity to format/parser and endpoint. The caller still owns and injects
// the HTTP transport policy (timeouts, TLS, proxies and lifecycle).
func NewDefaultLoaders(client HTTPDoer) (LoaderRegistry, error) {
	if isNilHTTPDoer(client) {
		return nil, fmt.Errorf("source sync: HTTP client is required")
	}
	return NewLoaderRegistry(LoaderRegistry{
		db.SourceEasyList:       easy_list.NewAdBlockLoader(client, easy_list.EasyListURL),
		db.SourceRuAdList:       easy_list.NewAdBlockLoader(client, easy_list.RuAdListURL),
		db.SourceAdGuardRussian: easy_list.NewAdBlockLoader(client, easy_list.AdGuardRussianURL),
		db.SourceStevenBlack:    NewHostsLoader(client, StevenBlackURL),
		db.SourceHaGeZiMulti:    NewHostsLoader(client, HaGeZiMultiURL),
	})
}
