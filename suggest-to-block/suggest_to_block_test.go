package suggest_to_block

import (
	"errors"
	"testing"
	"time"

	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/filter"
	filter_cache "github.com/alextorq/dns-filter/filter/cache"
	filter_bloom "github.com/alextorq/dns-filter/filter/filter"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"github.com/alextorq/dns-filter/utils"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type silentLog struct{}

func (silentLog) Info(args ...any)  {}
func (silentLog) Debug(args ...any) {}
func (silentLog) Error(err error)   {}

type harness struct {
	t            *testing.T
	conn         *gorm.DB
	module       *Module
	filterModule *filter.Module
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlConn, err := conn.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlConn.SetMaxOpenConns(1)
	if err := conn.AutoMigrate(
		&blocked_domain_db.BlockList{},
		&blocked_domain_db.BlockListReason{},
		&traffic_db.DomainTraffic{},
		&suggest_to_block_db.SuggestBlock{},
		&suggest_to_block_db.SuggestBlockReason{},
		&source_db.Source{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	blockRepo := blocked_domain_db.NewRepo(conn)
	// The suggest candidate pool now comes from domain_traffic (domains ever
	// forwarded upstream) via the traffic AllowFilterAdapter — the AllowRepo
	// port is unchanged, only its backing table moved (Step 3 of the migration).
	allowRepo := traffic_db.NewAllowFilterAdapter(traffic_db.NewRepo(conn))
	sourceRepo := source_db.NewRepo(conn)
	suggestRepo := suggest_to_block_db.NewRepo(conn)

	conf := &config.Config{}
	conf.Enabled.Store(true)
	log := silentLog{}

	bloom := &filter_bloom.Filter{}
	bloom.UpdateFilter(nil) // initialise so DomainExist is safe
	cache := filter_cache.NewCacheWithMetrics(1500)
	filterModule := filter.NewModule(blockRepo, bloom, cache, conf, log)

	module := NewModule(blockRepo, allowRepo, sourceRepo, filterModule, suggestRepo, log)
	return &harness{t: t, conn: conn, module: module, filterModule: filterModule}
}

func (h *harness) setSourceActive(name source_db.BlockListSource, active bool) {
	h.t.Helper()
	var s source_db.Source
	err := h.conn.Where("name = ?", name).First(&s).Error
	if err == nil {
		s.Active = active
		if err := h.conn.Save(&s).Error; err != nil {
			h.t.Fatalf("update source %s: %v", name, err)
		}
		return
	}
	if err := h.conn.Create(&source_db.Source{Name: name, Active: active}).Error; err != nil {
		h.t.Fatalf("seed source %s: %v", name, err)
	}
}

func (h *harness) seedBlocked(domain string) {
	h.t.Helper()
	h.seedBlockedWithSource(domain, source_db.SourceUser.String())
}

func (h *harness) seedBlockedWithSource(domain, src string) {
	h.t.Helper()
	// block_lists всегда хранит домены в канонической FQDN-форме (#30) —
	// сид-хелпер обязан это повторять, иначе DomainNotExist в Collect не
	// распознает «уже заблокирован» и создаст дубликат.
	if err := h.conn.Create(&blocked_domain_db.BlockList{
		Url:    utils.CanonicalDomain(domain),
		Active: true,
		Source: src,
	}).Error; err != nil {
		h.t.Fatalf("seed blocked %s: %v", domain, err)
	}
}

// seedAllowed records the domain as forwarded-upstream traffic (blocked=false),
// which is exactly the candidate pool the traffic AllowFilterAdapter exposes to
// the suggest collector. Mirrors a single device querying the domain once.
func (h *harness) seedAllowed(domain string) {
	h.t.Helper()
	if err := h.conn.Create(&traffic_db.DomainTraffic{
		ClientKind:  "ip",
		ClientValue: "10.0.0.1",
		ClientIP:    "10.0.0.1",
		Domain:      domain,
		Blocked:     false,
		Day:         time.Now().Truncate(24 * time.Hour),
		Count:       1,
		LastSeen:    time.Now(),
	}).Error; err != nil {
		h.t.Fatalf("seed allowed %s: %v", domain, err)
	}
}

// TestCollect_AutoBlocksSubdomainOfBlocked covers the reason-based gate
// (variant 2): a domain whose final score is between
// ThresholdToSuggestBlocking and ThresholdToAutoBlock — i.e. high enough
// to reach CollectSuggest's output but NOT high enough to satisfy
// variant 1's score gate — must still be auto-promoted because it carries
// a CodeSubdomainOfBlocked reason.
func TestCollect_AutoBlocksSubdomainOfBlocked(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	const allowed = "x8z7c4kqjfpw9.example.com"
	h.seedBlocked("example.com")
	h.seedAllowed(allowed)

	suggestions := collect.CollectSuggest([]string{"example.com"}, []string{allowed})
	if len(suggestions) != 1 {
		t.Fatalf("setup invariant: expected 1 suggestion, got %d (%+v)", len(suggestions), suggestions)
	}
	if suggestions[0].Score >= collect.ThresholdToAutoBlock {
		t.Fatalf("setup invariant: score %d >= ThresholdToAutoBlock %d — this test no longer isolates the reason gate",
			suggestions[0].Score, collect.ThresholdToAutoBlock)
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// CreateDomain пишет домен в канонической FQDN-форме (#30), поэтому в
	// block_lists он лежит с точкой на конце независимо от формы кандидата.
	var blockEntry blocked_domain_db.BlockList
	if err := h.conn.Where("url = ?", utils.CanonicalDomain(allowed)).First(&blockEntry).Error; err != nil {
		t.Fatalf("expected %s auto-blocked, lookup failed: %v", allowed, err)
	}
	if blockEntry.Source != source_db.SourceAutoBlocked.String() {
		t.Errorf("Source=%q, want %q", blockEntry.Source, source_db.SourceAutoBlocked)
	}

	var suggestCount int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).
		Where("domain = ?", allowed).Count(&suggestCount)
	if suggestCount != 0 {
		t.Errorf("auto-blocked domain must not also land in suggest table, got %d rows", suggestCount)
	}
}

// TestCollect_AutoBlockPersistsReasons — happy-path #95: при авто-блокировке
// reason-коды кандидата (code + match) сохраняются в block_list_reasons в
// одной транзакции с block_lists и доступны из БД без логов.
func TestCollect_AutoBlockPersistsReasons(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	const allowed = "x8z7c4kqjfpw9.example.com"
	h.seedBlocked("example.com")
	h.seedAllowed(allowed)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var entry blocked_domain_db.BlockList
	if err := h.conn.Preload("Reasons").
		Where("url = ?", utils.CanonicalDomain(allowed)).First(&entry).Error; err != nil {
		t.Fatalf("lookup auto-blocked domain: %v", err)
	}
	if len(entry.Reasons) == 0 {
		t.Fatal("auto-blocked domain has no reasons stored — #95 not satisfied")
	}

	var subdomainReason *blocked_domain_db.BlockListReason
	for i := range entry.Reasons {
		if entry.Reasons[i].BlockListID != entry.ID {
			t.Errorf("reason %q FK=%d, want %d", entry.Reasons[i].Code,
				entry.Reasons[i].BlockListID, entry.ID)
		}
		if entry.Reasons[i].Code == collect.CodeSubdomainOfBlocked {
			subdomainReason = &entry.Reasons[i]
		}
	}
	if subdomainReason == nil {
		t.Fatalf("expected a %s reason, got %+v", collect.CodeSubdomainOfBlocked, entry.Reasons)
	}
	if subdomainReason.MatchValue != "example.com" {
		t.Errorf("subdomain_of_blocked match=%q, want example.com", subdomainReason.MatchValue)
	}
}

// TestCollect_AutoBlockReasonsIdempotent — негатив #95: повторный прогон
// Collect на тех же данных не плодит дубли строк в block_list_reasons (домен
// уже в blocklist → CreateDomain возвращает ErrDomainAlreadyExists).
func TestCollect_AutoBlockReasonsIdempotent(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	h.seedBlocked("example.com")
	h.seedAllowed("x8z7c4kqjfpw9.example.com")

	if err := h.module.Collect(); err != nil {
		t.Fatalf("first Collect: %v", err)
	}
	var afterFirst int64
	h.conn.Model(&blocked_domain_db.BlockListReason{}).Count(&afterFirst)
	if afterFirst == 0 {
		t.Fatal("setup invariant: first Collect stored no reasons")
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("second Collect: %v", err)
	}
	var afterSecond int64
	h.conn.Model(&blocked_domain_db.BlockListReason{}).Count(&afterSecond)
	if afterSecond != afterFirst {
		t.Errorf("block_list_reasons grew on second Collect: %d → %d (must be idempotent)",
			afterFirst, afterSecond)
	}
}

// TestCollect_AutoBlockDisabled_FallsThroughToSuggest is the kill-switch
// контракт: если оператор выключил источник AutoBlocked через UI, Collect
// НЕ должен писать ничего в block_lists, даже для кандидатов, которые иначе
// прошли бы ShouldAutoBlock. Они должны мирно осесть в suggest_blocks под
// ручной разбор, а bloom-фильтр — остаться нетронутым.
func TestCollect_AutoBlockDisabled_FallsThroughToSuggest(t *testing.T) {
	h := newHarness(t)

	const allowed = "x8z7c4kqjfpw9.example.com"
	h.seedBlocked("example.com")
	h.seedAllowed(allowed)

	suggestions := collect.CollectSuggest([]string{"example.com"}, []string{allowed})
	if len(suggestions) != 1 || !collect.ShouldAutoBlock(suggestions[0]) {
		t.Fatalf("setup invariant: expected exactly 1 auto-block candidate, got %+v", suggestions)
	}

	h.setSourceActive(source_db.SourceAutoBlocked, false)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var inBlock int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", utils.CanonicalDomain(allowed)).Count(&inBlock)
	if inBlock != 0 {
		t.Errorf("AutoBlocked source is disabled but Collect wrote %d block_lists rows for %s — kill-switch ignored", inBlock, allowed)
	}

	var suggest suggest_to_block_db.SuggestBlock
	if err := h.conn.Preload("Reasons").Where("domain = ?", allowed).First(&suggest).Error; err != nil {
		t.Fatalf("expected %s to land in suggest_blocks when auto-block is off: %v", allowed, err)
	}
	if len(suggest.Reasons) == 0 {
		t.Error("expected reasons attached to suggest row, got none")
	}

	if h.filterModule.CheckExist(allowed) {
		t.Errorf("bloom must stay clean when auto-block is disabled, but %s is visible — UpdateFromDb was called", allowed)
	}
}

// TestCollect_AutoBlockSourceQueryFails_FailClosed закрепляет контракт
// «при ошибке source_db.IsActive Collect не блочит автоматически»: если БД
// не отвечает (имитируем дропом таблицы sources), вся ветка auto-promote
// выключается — иначе транзиентная проблема превращалась бы в тихий обход
// kill-switch'а. Дополнительно проверяем, что сам Collect не падает.
func TestCollect_AutoBlockSourceQueryFails_FailClosed(t *testing.T) {
	h := newHarness(t)

	const allowed = "x8z7c4kqjfpw9.example.com"
	h.seedBlocked("example.com")
	h.seedAllowed(allowed)

	if err := h.conn.Migrator().DropTable(&source_db.Source{}); err != nil {
		t.Fatalf("drop sources: %v", err)
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect must not surface IsActive error to caller, got: %v", err)
	}

	var inBlock int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", utils.CanonicalDomain(allowed)).Count(&inBlock)
	if inBlock != 0 {
		t.Errorf("auto-block must fail closed on IsActive error, but %d rows landed in block_lists", inBlock)
	}

	var inSuggest int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Where("domain = ?", allowed).Count(&inSuggest)
	if inSuggest != 1 {
		t.Errorf("expected suggest_blocks to still receive the candidate, got %d rows", inSuggest)
	}

	if h.filterModule.CheckExist(allowed) {
		t.Errorf("bloom must stay clean when auto-block fails closed, but %s is visible", allowed)
	}
}

// TestCollect_AutoBlocksByScoreThreshold covers the score-based gate
// (variant 1): a domain whose accumulated score clears
// ThresholdToAutoBlock but has no subdomain-of-blocked reason still gets
// auto-promoted.
func TestCollect_AutoBlocksByScoreThreshold(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	const allowed = "tracker.lzkdngfvtcwspbqxhrjm.deadbeef0123456789ab.paypa1.com"
	h.seedAllowed(allowed)

	suggestions := collect.CollectSuggest(nil, []string{allowed})
	if len(suggestions) != 1 {
		t.Fatalf("setup invariant: expected 1 collected suggestion, got %d (%+v)", len(suggestions), suggestions)
	}
	for _, r := range suggestions[0].Reasons {
		if r.Code == collect.CodeSubdomainOfBlocked {
			t.Fatalf("setup invariant: suggestion must not contain subdomain-of-blocked reason, got %+v", suggestions[0].Reasons)
		}
	}
	if suggestions[0].Score < collect.ThresholdToAutoBlock {
		t.Fatalf("setup invariant: collected score %d below ThresholdToAutoBlock %d (test no longer exercises score gate); reasons=%+v",
			suggestions[0].Score, collect.ThresholdToAutoBlock, suggestions[0].Reasons)
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var entry blocked_domain_db.BlockList
	if err := h.conn.Where("url = ?", utils.CanonicalDomain(allowed)).First(&entry).Error; err != nil {
		t.Fatalf("expected %s auto-blocked, lookup failed: %v", allowed, err)
	}
	if entry.Source != source_db.SourceAutoBlocked.String() {
		t.Errorf("Source=%q, want %q", entry.Source, source_db.SourceAutoBlocked)
	}
}

// TestCollect_BelowAutoBlockGate_StaysInSuggest is the negative case: a
// suggestion that clears ThresholdToSuggestBlocking but neither gate
// triggers — so it must land in the suggest table for manual review and
// NOT in the blocklist.
func TestCollect_BelowAutoBlockGate_StaysInSuggest(t *testing.T) {
	h := newHarness(t)

	const allowed = "x8z7c4kqjfpw9.example.click"
	h.seedAllowed(allowed)

	suggestions := collect.CollectSuggest(nil, []string{allowed})
	if len(suggestions) != 1 {
		t.Fatalf("setup invariant: expected 1 suggestion, got %d (%+v)", len(suggestions), suggestions)
	}
	if suggestions[0].Score >= collect.ThresholdToAutoBlock {
		t.Fatalf("setup invariant: score %d should be below ThresholdToAutoBlock %d",
			suggestions[0].Score, collect.ThresholdToAutoBlock)
	}
	for _, r := range suggestions[0].Reasons {
		if r.Code == collect.CodeSubdomainOfBlocked {
			t.Fatalf("setup invariant: must not have subdomain-of-blocked reason, got %+v", suggestions[0].Reasons)
		}
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var blockCount int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", utils.CanonicalDomain(allowed)).Count(&blockCount)
	if blockCount != 0 {
		t.Errorf("domain must not be auto-blocked, got %d blocklist rows", blockCount)
	}

	var suggest suggest_to_block_db.SuggestBlock
	if err := h.conn.Preload("Reasons").Where("domain = ?", allowed).First(&suggest).Error; err != nil {
		t.Fatalf("expected suggest row for %s: %v", allowed, err)
	}
	if len(suggest.Reasons) == 0 {
		t.Error("expected reasons attached to suggest row, got none")
	}
}

// TestCollect_AutoBlockUpdatesBloomFilter pins the *user-visible* effect of
// the feature: после Collect авто-заблокированный домен реально становится
// видимым через filter.CheckExist.
func TestCollect_AutoBlockUpdatesBloomFilter(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	const allowed = "x8z7c4kqjfpw9.example.com"
	h.seedBlocked("example.com")
	h.seedAllowed(allowed)

	if h.filterModule.CheckExist(allowed) {
		t.Fatalf("setup invariant: %s already known to filter before Collect", allowed)
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if !h.filterModule.CheckExist(allowed) {
		t.Fatalf("expected %s to be visible to filter after auto-block — UpdateFromDb wasn't called?", allowed)
	}
}

// TestCollect_NoAutoBlock_SkipsFilterRebuild — обратная сторона: если в
// батче не было ни одного auto-block, filter.UpdateFromDb() не должен
// вызываться. Прокси-assertion: bloom-знание про допущенный домен не
// меняется после Collect.
func TestCollect_NoAutoBlock_SkipsFilterRebuild(t *testing.T) {
	h := newHarness(t)

	const allowed = "x8z7c4kqjfpw9.example.click"
	h.seedAllowed(allowed)

	const orphan = "should-not-be-loaded.example"
	h.seedBlocked(orphan)

	if h.filterModule.CheckExist(orphan) {
		t.Fatalf("setup invariant: orphan domain already in bloom — test cannot distinguish rebuild")
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if h.filterModule.CheckExist(orphan) {
		t.Fatalf("UpdateFromDb was called even though nothing was auto-blocked — orphan leaked into bloom")
	}
}

// TestCollect_MixedBatch проверяет, что один прогон Collect корректно
// разводит три категории доменов одновременно: auto-block, suggest-only,
// и фоновый «уже заблокирован другим Source».
func TestCollect_MixedBatch(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	h.seedBlocked("example.com")
	const autoA = "x8z7c4kqjfpw9.example.com"
	const autoB = "lzkdngfvtcwspbqxhrjm.example.com"
	h.seedAllowed(autoA)
	h.seedAllowed(autoB)

	const suggested = "x8z7c4kqjfpw9.other.click"
	h.seedAllowed(suggested)

	const preBlocked = "x8z7c4kqjfpw9.preblocked.com"
	h.seedBlocked("preblocked.com")
	h.seedBlockedWithSource(preBlocked, source_db.SourceHaGeZiMulti.String())
	h.seedAllowed(preBlocked)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	for _, d := range []string{autoA, autoB} {
		var entry blocked_domain_db.BlockList
		if err := h.conn.Where("url = ?", utils.CanonicalDomain(d)).First(&entry).Error; err != nil {
			t.Errorf("expected %s auto-blocked: %v", d, err)
			continue
		}
		if entry.Source != source_db.SourceAutoBlocked.String() {
			t.Errorf("%s Source=%q, want %q", d, entry.Source, source_db.SourceAutoBlocked)
		}
	}

	var pre blocked_domain_db.BlockList
	if err := h.conn.Where("url = ?", utils.CanonicalDomain(preBlocked)).First(&pre).Error; err != nil {
		t.Fatalf("preBlocked vanished from blocklist: %v", err)
	}
	if pre.Source != source_db.SourceHaGeZiMulti.String() {
		t.Errorf("preBlocked Source overwritten: got %q, want %q (Collect must not upgrade existing Source)",
			pre.Source, source_db.SourceHaGeZiMulti)
	}

	var preInSuggest int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Where("domain = ?", preBlocked).Count(&preInSuggest)
	if preInSuggest != 0 {
		t.Errorf("already-blocked domain leaked into suggest, got %d rows", preInSuggest)
	}

	var sugInBlock int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", utils.CanonicalDomain(suggested)).Count(&sugInBlock)
	if sugInBlock != 0 {
		t.Errorf("suggest-only domain leaked into blocklist, got %d rows", sugInBlock)
	}
	var sugRow suggest_to_block_db.SuggestBlock
	if err := h.conn.Where("domain = ?", suggested).First(&sugRow).Error; err != nil {
		t.Errorf("suggest-only domain missing from suggest table: %v", err)
	}
}

// TestCollect_Idempotent повторно запускает Collect на тех же данных и
// проверяет, что ничего не дублируется и Collect возвращает nil.
func TestCollect_Idempotent(t *testing.T) {
	h := newHarness(t)
	h.setSourceActive(source_db.SourceAutoBlocked, true)

	h.seedBlocked("example.com")
	h.seedAllowed("x8z7c4kqjfpw9.example.com")
	h.seedAllowed("x8z7c4kqjfpw9.other.click")

	if err := h.module.Collect(); err != nil {
		t.Fatalf("first Collect: %v", err)
	}

	var blockedAfterFirst, suggestAfterFirst int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Count(&blockedAfterFirst)
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestAfterFirst)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("second Collect: %v", err)
	}

	var blockedAfterSecond, suggestAfterSecond int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Count(&blockedAfterSecond)
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestAfterSecond)

	if blockedAfterSecond != blockedAfterFirst {
		t.Errorf("blocklist grew on second Collect: %d → %d (auto-block must be idempotent)",
			blockedAfterFirst, blockedAfterSecond)
	}
	if suggestAfterSecond != suggestAfterFirst {
		t.Errorf("suggest_blocks grew on second Collect: %d → %d", suggestAfterFirst, suggestAfterSecond)
	}
}

// TestCollect_EmptyInput — happy-path no-op: ни blocked, ни allowed.
func TestCollect_EmptyInput(t *testing.T) {
	h := newHarness(t)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect on empty DB returned error: %v", err)
	}

	var blockedCount, suggestCount int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Count(&blockedCount)
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestCount)
	if blockedCount != 0 || suggestCount != 0 {
		t.Errorf("expected empty DB after Collect on empty input, got blocked=%d suggest=%d",
			blockedCount, suggestCount)
	}
}

// TestCollect_AllowedButNoSignals — есть allowed-домены, но ни один не
// собирает score >= ThresholdToSuggestBlocking.
func TestCollect_AllowedButNoSignals(t *testing.T) {
	h := newHarness(t)

	h.seedAllowed("plain.example")
	h.seedAllowed("another-plain.example")

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var blockedCount, suggestCount int64
	h.conn.Model(&blocked_domain_db.BlockList{}).Count(&blockedCount)
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestCount)
	if blockedCount != 0 {
		t.Errorf("nothing should be auto-blocked, got %d", blockedCount)
	}
	if suggestCount != 0 {
		t.Errorf("nothing should be suggested, got %d", suggestCount)
	}
}

// TestCollect_BlockRepoError_PropagatesAndSkipsRest is the negative case:
// if loading the blocked list fails, Collect must surface the error and
// not proceed to the allow-side fetch / source gate (those would happen on
// a fresh DB with possibly different state and confuse operators).
func TestCollect_BlockRepoError_PropagatesAndSkipsRest(t *testing.T) {
	h := newHarness(t)
	if err := h.conn.Migrator().DropTable(&blocked_domain_db.BlockList{}); err != nil {
		t.Fatalf("drop block_lists: %v", err)
	}

	if err := h.module.Collect(); err == nil {
		t.Fatal("expected Collect to surface blockRepo error, got nil")
	}
}

// fakeInspectQueue records UpsertCandidate calls so tests can assert exactly
// which weak-band domains Collect routed to the reputation queue.
type fakeInspectQueue struct {
	err   error // when set, UpsertCandidate fails — to exercise the non-fatal path
	calls []struct {
		domain      string
		score       int
		reasonsJSON string
	}
}

func (f *fakeInspectQueue) UpsertCandidate(domain string, lexicalScore int, reasonsJSON string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, struct {
		domain      string
		score       int
		reasonsJSON string
	}{domain, lexicalScore, reasonsJSON})
	return nil
}

// TestCollect_InspectQueueDisabled_NoQueueNoChange is the golden regression: with
// the inspect queue unwired (feature off, the default), a weak-band domain
// (score in [10,30)) is silently dropped exactly as before — it reaches neither
// the suggest list nor the blocklist, and Collect behaviour is unchanged.
func TestCollect_InspectQueueDisabled_NoQueueNoChange(t *testing.T) {
	h := newHarness(t)

	const weak = "ads.shop.xyz" // score 10 — inspect band, below suggest threshold
	h.seedAllowed(weak)

	// Sanity: this domain is in the weak band, not the suggest band.
	scored := collect.ScoreCandidates(nil, []string{weak})
	if len(scored) != 1 || scored[0].Score >= collect.ThresholdToSuggestBlocking {
		t.Fatalf("setup invariant: %s must be weak-band, got %+v", weak, scored)
	}

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var suggestCount, blockCount int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Where("domain = ?", weak).Count(&suggestCount)
	h.conn.Model(&blocked_domain_db.BlockList{}).Where("url = ?", utils.CanonicalDomain(weak)).Count(&blockCount)
	if suggestCount != 0 || blockCount != 0 {
		t.Errorf("feature off: weak domain must be dropped, got suggest=%d block=%d", suggestCount, blockCount)
	}
}

// TestCollect_InspectQueueEnabled_RoutesByBand pins the bucketing: with the
// queue wired, a weak-band domain is queued (and kept out of the suggest list),
// a strong domain goes to the suggest list (and is NOT queued), and a
// signalless domain goes nowhere.
func TestCollect_InspectQueueEnabled_RoutesByBand(t *testing.T) {
	h := newHarness(t)
	q := &fakeInspectQueue{}
	h.module.SetInspectQueue(q)

	const weak = "ads.shop.xyz"                  // ~10 → queue
	const strong = "x8z7c4kqjfpw9.example.click" // >=30 → suggest list
	const noise = "plain.example"                // 0 → nowhere
	h.seedAllowed(weak)
	h.seedAllowed(strong)
	h.seedAllowed(noise)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Weak domain queued exactly once, with its lexical score and a non-empty
	// reasons snapshot; never the strong or noise domain.
	if len(q.calls) != 1 {
		t.Fatalf("expected exactly 1 queued candidate, got %d (%+v)", len(q.calls), q.calls)
	}
	if q.calls[0].domain != weak {
		t.Errorf("queued domain = %q, want %q", q.calls[0].domain, weak)
	}
	if q.calls[0].score < collect.MinInspectCandidateScore || q.calls[0].score >= collect.ThresholdToSuggestBlocking {
		t.Errorf("queued score %d not in inspect band", q.calls[0].score)
	}
	// Exact wire form: canonical lowercase {"code":...} with "match" omitted when
	// empty — must match the shape the worker (M4) unmarshals and the db layer
	// stores. "ads" bad-keyword is scored before ".xyz" risky-TLD, so the order
	// is deterministic.
	const wantReasons = `[{"code":"bad_keywords"},{"code":"risky_tld"}]`
	if q.calls[0].reasonsJSON != wantReasons {
		t.Errorf("queued reasons snapshot = %q, want %q", q.calls[0].reasonsJSON, wantReasons)
	}

	// Weak domain must NOT also be in the suggest list.
	var weakInSuggest int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Where("domain = ?", weak).Count(&weakInSuggest)
	if weakInSuggest != 0 {
		t.Errorf("weak domain leaked into suggest list, got %d rows", weakInSuggest)
	}

	// Strong domain in suggest list, and never queued.
	var strongInSuggest int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Where("domain = ?", strong).Count(&strongInSuggest)
	if strongInSuggest != 1 {
		t.Errorf("strong domain must be in suggest list, got %d rows", strongInSuggest)
	}
	for _, c := range q.calls {
		if c.domain == strong || c.domain == noise {
			t.Errorf("%q must not be queued for inspection", c.domain)
		}
	}
}

// TestCollect_InspectQueueError_DoesNotBreakBatch is the negative case for the
// queue path: a failing UpsertCandidate is logged but must not abort Collect —
// the strong-band domain still reaches the suggest list and Collect returns nil.
func TestCollect_InspectQueueError_DoesNotBreakBatch(t *testing.T) {
	h := newHarness(t)
	q := &fakeInspectQueue{err: errors.New("queue down")}
	h.module.SetInspectQueue(q)

	const weak = "ads.shop.xyz"                  // would queue, but the queue errors
	const strong = "x8z7c4kqjfpw9.example.click" // must still reach the suggest list
	h.seedAllowed(weak)
	h.seedAllowed(strong)

	if err := h.module.Collect(); err != nil {
		t.Fatalf("a queue error must not surface from Collect, got: %v", err)
	}

	var strongInSuggest int64
	h.conn.Model(&suggest_to_block_db.SuggestBlock{}).Where("domain = ?", strong).Count(&strongInSuggest)
	if strongInSuggest != 1 {
		t.Errorf("strong domain must still be suggested despite queue error, got %d rows", strongInSuggest)
	}
}
