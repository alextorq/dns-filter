package suggest_to_block

import (
	"testing"

	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/filter"
	filter_cache "github.com/alextorq/dns-filter/filter/cache"
	filter_bloom "github.com/alextorq/dns-filter/filter/filter"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
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
		&blocked_domain_db.BlockDomainEvent{},
		&allow_domain_db.AllowDomainEvent{},
		&suggest_to_block_db.SuggestBlock{},
		&suggest_to_block_db.SuggestBlockReason{},
		&source_db.Source{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	blockRepo := blocked_domain_db.NewRepo(conn)
	allowRepo := allow_domain_db.NewRepo(conn)
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

func (h *harness) seedAllowed(domain string) {
	h.t.Helper()
	if err := h.conn.Create(&allow_domain_db.AllowDomainEvent{
		Domain: domain,
		Active: true,
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
