package sync

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/alextorq/dns-filter/source/db"
)

type recordedDelete struct {
	source string
	keep   []string
}

// fakeBlockWriter records DeleteDNSRecordsBySourceNotIn calls so prune logic
// can be asserted without a real DB.
type fakeBlockWriter struct {
	creates  int
	deletes  []recordedDelete
	delErr   error
	onDelete func()
}

func (f *fakeBlockWriter) CreateDNSRecordsByDomainsContext(context.Context, []string, string) error {
	f.creates++
	return nil
}

func (f *fakeBlockWriter) DeleteDNSRecordsBySourceNotInContext(_ context.Context, source string, keep []string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deletes = append(f.deletes, recordedDelete{source: source, keep: append([]string(nil), keep...)})
	if f.onDelete != nil {
		f.onDelete()
	}
	return nil
}

type silentLogger struct{}

func (silentLogger) Debug(_ ...any) {}
func (silentLogger) Error(_ error)  {}

type fakeSourceLister struct{ calls int }

func (f *fakeSourceLister) GetAllActive() ([]db.Source, error) {
	f.calls++
	return nil, nil
}

func TestSync_PreCanceledSkipsReadsAndWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sources := &fakeSourceLister{}
	blocks := &fakeBlockWriter{}

	err := Sync(ctx, sources, blocks, silentLogger{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Sync error = %v, want context.Canceled", err)
	}
	if sources.calls != 0 || blocks.creates != 0 || len(blocks.deletes) != 0 {
		t.Fatalf("pre-canceled Sync did work: reads=%d creates=%d deletes=%d", sources.calls, blocks.creates, len(blocks.deletes))
	}
}

// sortedSet returns the unique elements of in, sorted — for order-independent
// comparison of a keep set.
func sortedSet(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Регрессия на C1: домен, лежащий в БД под одним источником, но присутствующий
// в свежем наборе ДРУГОГО источника, не должен удаляться. pruneVanishedDomains
// обязан диффить каждый источник против union всех свежих наборов, поэтому в
// keep, переданный для каждого источника, попадают и домены чужих листов.
func TestPruneVanishedDomains_PrunesAgainstUnionOfAllSources(t *testing.T) {
	list := []DomainBySource{
		{Source: db.SourceEasyList, Domains: []string{"shared.example", "only-easy.example"}},
		{Source: db.SourceRuAdList, Domains: []string{"shared.example", "only-ru.example"}},
	}
	w := &fakeBlockWriter{}

	if err := pruneVanishedDomains(context.Background(), list, true, w, silentLogger{}); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if len(w.deletes) != 2 {
		t.Fatalf("expected 2 prune calls, got %d (%+v)", len(w.deletes), w.deletes)
	}
	wantKeep := []string{"only-easy.example", "only-ru.example", "shared.example"}
	for _, d := range w.deletes {
		if got := sortedSet(d.keep); !slices.Equal(got, wantKeep) {
			t.Errorf("source %s: keep=%v, want union %v — домен чужого листа обязан быть в keep", d.source, got, wantKeep)
		}
	}
}

// Если хоть один источник не скачался, union неполон — prune целиком
// пропускается, иначе домены упавшего источника были бы удалены.
func TestPruneVanishedDomains_SkippedWhenSyncIncomplete(t *testing.T) {
	list := []DomainBySource{{Source: db.SourceEasyList, Domains: []string{"a.example"}}}
	w := &fakeBlockWriter{}

	if err := pruneVanishedDomains(context.Background(), list, false, w, silentLogger{}); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(w.deletes) != 0 {
		t.Fatalf("incomplete sync must skip prune entirely, got %d delete calls", len(w.deletes))
	}
}

// Источник, распарсенный в пустой набор, не пруним: пустой парсинг — скорее
// мусорный ответ, чем реально опустевший лист. Остальные источники пруним.
func TestPruneVanishedDomains_SkipsEmptySource(t *testing.T) {
	list := []DomainBySource{
		{Source: db.SourceEasyList, Domains: []string{"a.example"}},
		{Source: db.SourceRuAdList, Domains: nil},
	}
	w := &fakeBlockWriter{}

	if err := pruneVanishedDomains(context.Background(), list, true, w, silentLogger{}); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(w.deletes) != 1 || w.deletes[0].source != db.SourceEasyList.String() {
		t.Fatalf("expected prune only for EasyList, got %+v", w.deletes)
	}
}

// Ошибка удаления в одном источнике прерывает весь батч и пробрасывается.
func TestPruneVanishedDomains_PropagatesDeleteError(t *testing.T) {
	list := []DomainBySource{{Source: db.SourceEasyList, Domains: []string{"a.example"}}}
	sentinel := errors.New("db down")
	w := &fakeBlockWriter{delErr: sentinel}

	if err := pruneVanishedDomains(context.Background(), list, true, w, silentLogger{}); !errors.Is(err, sentinel) {
		t.Fatalf("expected delete error propagated, got %v", err)
	}
}

func TestPruneVanishedDomains_CancelBetweenSourcesStopsFurtherDeletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	list := []DomainBySource{
		{Source: db.SourceEasyList, Domains: []string{"a.example"}},
		{Source: db.SourceRuAdList, Domains: []string{"b.example"}},
	}
	w := &fakeBlockWriter{onDelete: cancel}

	err := pruneVanishedDomains(ctx, list, true, w, silentLogger{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("prune error = %v, want context.Canceled", err)
	}
	if len(w.deletes) != 1 {
		t.Fatalf("delete calls = %d, want 1 before cancellation", len(w.deletes))
	}
}
