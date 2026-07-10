package source

import (
	"context"
	"testing"

	sync_sources "github.com/alextorq/dns-filter/source/business/use-cases/sync"
	"github.com/alextorq/dns-filter/source/db"
)

type fakeSourceRepo struct {
	seeded bool
	items  []db.Source
}

func (f *fakeSourceRepo) Seed() { f.seeded = true }

func (f *fakeSourceRepo) GetAllActive() ([]db.Source, error) { return f.items, nil }

type fakeBlockRepo struct{ created []string }

func (f *fakeBlockRepo) CreateDNSRecordsByDomainsContext(_ context.Context, domains []string, _ string) error {
	f.created = append(f.created, domains...)
	return nil
}

func (*fakeBlockRepo) DeleteDNSRecordsBySourceNotInContext(context.Context, string, []string) error {
	return nil
}

type moduleLogger struct{}

func (moduleLogger) Info(...any)  {}
func (moduleLogger) Debug(...any) {}
func (moduleLogger) Error(error)  {}

type moduleLoader func(context.Context) ([]string, error)

func (f moduleLoader) Load(ctx context.Context) ([]string, error) { return f(ctx) }

func completeLoaderRegistry(loader sync_sources.Loader) sync_sources.LoaderRegistry {
	return sync_sources.LoaderRegistry{
		db.SourceEasyList:       loader,
		db.SourceRuAdList:       loader,
		db.SourceAdGuardRussian: loader,
		db.SourceStevenBlack:    loader,
		db.SourceHaGeZiMulti:    loader,
	}
}

func TestModule_UsesConsumerOwnedRepoAndInjectedLoaders(t *testing.T) {
	repo := &fakeSourceRepo{items: []db.Source{{Name: db.SourceEasyList, Active: true}}}
	blocks := &fakeBlockRepo{}
	loaders := completeLoaderRegistry(moduleLoader(func(context.Context) ([]string, error) {
		return []string{"injected.example."}, nil
	}))
	module, err := NewModule(repo, blocks, loaders, moduleLogger{})
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}

	module.Seed()
	if !repo.seeded {
		t.Fatal("Seed did not use injected SourceRepo")
	}
	if err := module.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if len(blocks.created) != 1 || blocks.created[0] != "injected.example." {
		t.Fatalf("created domains = %v, want injected loader result", blocks.created)
	}
}

func TestNewModule_RejectsIncompleteRemoteLoaderRegistry(t *testing.T) {
	_, err := NewModule(&fakeSourceRepo{}, &fakeBlockRepo{}, sync_sources.LoaderRegistry{}, moduleLogger{})
	if err == nil {
		t.Fatal("NewModule accepted missing remote loaders")
	}
}

func TestNewModule_RejectsTypedNilDependencies(t *testing.T) {
	loader := moduleLoader(func(context.Context) ([]string, error) { return nil, nil })
	loaders := completeLoaderRegistry(loader)

	var repo *fakeSourceRepo
	if _, err := NewModule(repo, &fakeBlockRepo{}, loaders, moduleLogger{}); err == nil {
		t.Fatal("NewModule accepted typed-nil SourceRepo")
	}

	var blocks *fakeBlockRepo
	if _, err := NewModule(&fakeSourceRepo{}, blocks, loaders, moduleLogger{}); err == nil {
		t.Fatal("NewModule accepted typed-nil BlockWriter")
	}

	var log *moduleLogger
	if _, err := NewModule(&fakeSourceRepo{}, &fakeBlockRepo{}, loaders, log); err == nil {
		t.Fatal("NewModule accepted typed-nil Logger")
	}
}
