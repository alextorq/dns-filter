package suggest_to_block

import (
	"os"
	"testing"

	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/filter"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "suggest-collect-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	conn := app_db.GetConnection()
	if err := conn.AutoMigrate(
		&blocked_domain_db.BlockList{},
		&blocked_domain_db.BlockDomainEvent{},
		&allow_domain_db.AllowDomainEvent{},
		&suggest_to_block_db.SuggestBlock{},
		&suggest_to_block_db.SuggestBlockReason{},
	); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}
	if err := filter.UpdateFilterFromDb(); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func resetTables(t *testing.T) {
	t.Helper()
	conn := app_db.GetConnection()
	conn.Exec("DELETE FROM block_lists")
	conn.Exec("DELETE FROM block_domain_events")
	conn.Exec("DELETE FROM allow_domain_events")
	conn.Exec("DELETE FROM suggest_block_reasons")
	conn.Exec("DELETE FROM suggest_blocks")
}

func seedBlocked(t *testing.T, domain string) {
	t.Helper()
	seedBlockedWithSource(t, domain, source_db.SourceUser.String())
}

func seedBlockedWithSource(t *testing.T, domain, src string) {
	t.Helper()
	if err := app_db.GetConnection().Create(&blocked_domain_db.BlockList{
		Url:    domain,
		Active: true,
		Source: src,
	}).Error; err != nil {
		t.Fatalf("seed blocked %s: %v", domain, err)
	}
}

func seedAllowed(t *testing.T, domain string) {
	t.Helper()
	if err := app_db.GetConnection().Create(&allow_domain_db.AllowDomainEvent{
		Domain: domain,
		Active: true,
	}).Error; err != nil {
		t.Fatalf("seed allowed %s: %v", domain, err)
	}
}

// TestCollect_AutoBlocksSubdomainOfBlocked covers the reason-based gate
// (variant 2): a domain whose final score is between
// ThresholdToSuggestBlocking and ThresholdToAutoBlock — i.e. high enough
// to reach CollectSuggest's output but NOT high enough to satisfy
// variant 1's score gate — must still be auto-promoted because it carries
// a CodeSubdomainOfBlocked reason. The fixture domain is the same combo
// that TestCollectSuggest_AccumulatesEntropyAndSubdomain pins:
// entropy (+20) + subdomain (+20) = 40, so the assertion failure here
// uniquely identifies a regression in the variant-2 gate (not in scoring).
func TestCollect_AutoBlocksSubdomainOfBlocked(t *testing.T) {
	resetTables(t)

	const allowed = "x8z7c4kqjfpw9.example.com"
	seedBlocked(t, "example.com")
	seedAllowed(t, allowed)

	// Setup invariant: the suggestion lands between the two thresholds, so
	// it's ONLY the subdomain-of-blocked reason that can trip auto-block.
	suggestions := collect.CollectSuggest([]string{"example.com"}, []string{allowed})
	if len(suggestions) != 1 {
		t.Fatalf("setup invariant: expected 1 suggestion, got %d (%+v)", len(suggestions), suggestions)
	}
	if suggestions[0].Score >= collect.ThresholdToAutoBlock {
		t.Fatalf("setup invariant: score %d >= ThresholdToAutoBlock %d — this test no longer isolates the reason gate",
			suggestions[0].Score, collect.ThresholdToAutoBlock)
	}

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var blockEntry blocked_domain_db.BlockList
	if err := app_db.GetConnection().
		Where("url = ?", allowed).
		First(&blockEntry).Error; err != nil {
		t.Fatalf("expected %s auto-blocked, lookup failed: %v", allowed, err)
	}
	if blockEntry.Source != source_db.SourceAutoBlocked.String() {
		t.Errorf("Source=%q, want %q", blockEntry.Source, source_db.SourceAutoBlocked)
	}

	var suggestCount int64
	app_db.GetConnection().Model(&suggest_to_block_db.SuggestBlock{}).
		Where("domain = ?", allowed).Count(&suggestCount)
	if suggestCount != 0 {
		t.Errorf("auto-blocked domain must not also land in suggest table, got %d rows", suggestCount)
	}
}

// TestCollect_AutoBlocksByScoreThreshold covers the score-based gate
// (variant 1): a domain whose accumulated score clears
// ThresholdToAutoBlock but has no subdomain-of-blocked reason still gets
// auto-promoted. We synthesize this via brand-impersonation +
// similar-to-blocked + risky-TLD on a same-depth blocked sibling.
func TestCollect_AutoBlocksByScoreThreshold(t *testing.T) {
	resetTables(t)

	// Combo that hits ThresholdToAutoBlock without any blocked-parent:
	//   - apex `paypa1.com` ≈ `paypal.com` → brand impersonation (+25)
	//   - high-entropy / all-consonant label                      (+20)
	//   - hex-UUID-looking label                                  (+10)
	//   - 7+ digit run inside the hex label                       (+5)
	//   - bad-keyword "tracker"                                   (+5)
	// Total = 65, comfortably above 60 and entirely score-driven.
	const allowed = "tracker.lzkdngfvtcwspbqxhrjm.deadbeef0123456789ab.paypa1.com"
	seedAllowed(t, allowed)

	// Sanity-check that the synthetic suggestion clears ThresholdToAutoBlock
	// without any subdomain-of-blocked reason — otherwise the test is silently
	// exercising the wrong gate.
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

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var entry blocked_domain_db.BlockList
	if err := app_db.GetConnection().
		Where("url = ?", allowed).
		First(&entry).Error; err != nil {
		t.Fatalf("expected %s auto-blocked, lookup failed: %v", allowed, err)
	}
	if entry.Source != source_db.SourceAutoBlocked.String() {
		t.Errorf("Source=%q, want %q", entry.Source, source_db.SourceAutoBlocked)
	}
}

// TestCollect_BelowAutoBlockGate_StaysInSuggest is the negative case: a
// suggestion that clears ThresholdToSuggestBlocking but neither gate
// triggers — so it must land in the suggest table for manual review and
// NOT in the blocklist. Without this test, an over-eager auto-block rule
// (e.g. lowering ThresholdToAutoBlock) would slip past CI.
func TestCollect_BelowAutoBlockGate_StaysInSuggest(t *testing.T) {
	resetTables(t)

	// Random-looking hash label triggers suspicious-entropy (+20). On a
	// risky TLD that totals +25 — clears suggest (30) only when combined
	// with risky-TLD (+5). Crucially: no subdomain-of-blocked reason.
	const allowed = "x8z7c4kqjfpw9.example.click"
	seedAllowed(t, allowed)

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

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var blockCount int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).
		Where("url = ?", allowed).Count(&blockCount)
	if blockCount != 0 {
		t.Errorf("domain must not be auto-blocked, got %d blocklist rows", blockCount)
	}

	var suggest suggest_to_block_db.SuggestBlock
	if err := app_db.GetConnection().
		Preload("Reasons").
		Where("domain = ?", allowed).
		First(&suggest).Error; err != nil {
		t.Fatalf("expected suggest row for %s: %v", allowed, err)
	}
	if len(suggest.Reasons) == 0 {
		t.Error("expected reasons attached to suggest row, got none")
	}
}

// TestCollect_AutoBlockUpdatesBloomFilter pins the *user-visible* effect of
// the feature: после Collect авто-заблокированный домен реально становится
// видимым через filter.CheckExist. Без этого assertion'а можно случайно
// сломать вызов filter.UpdateFilterFromDb (например, гейтом autoBlocked>0
// при подсчёте ошибок) — БД-ассерты будут зелёные, а DNS-хотпат продолжит
// резолвить домен.
func TestCollect_AutoBlockUpdatesBloomFilter(t *testing.T) {
	resetTables(t)
	// сбросить bloom от предыдущих тестов — иначе CheckExist может вернуть
	// true из-за артефактов чужого теста.
	if err := filter.UpdateFilterFromDb(); err != nil {
		t.Fatalf("reset bloom: %v", err)
	}

	const allowed = "x8z7c4kqjfpw9.example.com"
	seedBlocked(t, "example.com")
	seedAllowed(t, allowed)

	if filter.CheckExist(allowed) {
		t.Fatalf("setup invariant: %s already known to filter before Collect", allowed)
	}

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if !filter.CheckExist(allowed) {
		t.Fatalf("expected %s to be visible to filter after auto-block — UpdateFilterFromDb wasn't called?", allowed)
	}
}

// TestCollect_NoAutoBlock_SkipsFilterRebuild — обратная сторона: если в
// батче не было ни одного auto-block, filter.UpdateFilterFromDb() не должен
// вызываться. Прокси-assertion: bloom-знание про допущенный домен не
// меняется после Collect (то есть rebuild не «случайно» втянул что-то
// постороннее). Тест не вызывает rebuild сам, чтобы оставить bloom в
// known-empty состоянии — а после Collect он должен быть таким же.
func TestCollect_NoAutoBlock_SkipsFilterRebuild(t *testing.T) {
	resetTables(t)
	if err := filter.UpdateFilterFromDb(); err != nil {
		t.Fatalf("reset bloom: %v", err)
	}

	// Suggest-only домен (не auto-block-кандидат): см. TestCollect_BelowAutoBlockGate_StaysInSuggest.
	const allowed = "x8z7c4kqjfpw9.example.click"
	seedAllowed(t, allowed)

	// Параллельно вручную «загрязняем» blocklist домен, который должен был
	// бы попасть в bloom при принудительном rebuild. Если Collect зря
	// вызовет UpdateFilterFromDb, bloom втянет orphan-домен.
	const orphan = "should-not-be-loaded.example"
	seedBlocked(t, orphan)

	if filter.CheckExist(orphan) {
		t.Fatalf("setup invariant: orphan domain already in bloom — test cannot distinguish rebuild")
	}

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if filter.CheckExist(orphan) {
		t.Fatalf("UpdateFilterFromDb was called even though nothing was auto-blocked — orphan leaked into bloom")
	}
}

// TestCollect_MixedBatch проверяет, что один прогон Collect корректно
// разводит три категории доменов одновременно: auto-block, suggest-only,
// и фоновый «уже заблокирован другим Source» (наиболее частый prod-сценарий
// — пользователь добавил вручную через UI или импортнул из HaGeZi).
func TestCollect_MixedBatch(t *testing.T) {
	resetTables(t)

	// auto-block кандидаты (subdomain-of-blocked, score < 60).
	seedBlocked(t, "example.com")
	const autoA = "x8z7c4kqjfpw9.example.com"
	const autoB = "lzkdngfvtcwspbqxhrjm.example.com"
	seedAllowed(t, autoA)
	seedAllowed(t, autoB)

	// suggest-only (entropy + risky TLD, no subdomain reason, score < 60).
	const suggested = "x8z7c4kqjfpw9.other.click"
	seedAllowed(t, suggested)

	// already-blocked другим Source: тот же subdomain-of-blocked паттерн,
	// но домен уже сидит в blocklist как SourceHaGeZiMulti — Collect должен
	// вернуть autoBlockAlreadyExists, не upgrade'нуть Source, не уронить
	// батч и не положить домен в suggest.
	const preBlocked = "x8z7c4kqjfpw9.preblocked.com"
	seedBlocked(t, "preblocked.com")
	seedBlockedWithSource(t, preBlocked, source_db.SourceHaGeZiMulti.String())
	seedAllowed(t, preBlocked)

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// autoA, autoB — в blocklist с Source=AutoBlocked.
	for _, d := range []string{autoA, autoB} {
		var entry blocked_domain_db.BlockList
		if err := app_db.GetConnection().Where("url = ?", d).First(&entry).Error; err != nil {
			t.Errorf("expected %s auto-blocked: %v", d, err)
			continue
		}
		if entry.Source != source_db.SourceAutoBlocked.String() {
			t.Errorf("%s Source=%q, want %q", d, entry.Source, source_db.SourceAutoBlocked)
		}
	}

	// preBlocked — остался под исходным Source (Collect не должен «переаппрувить»).
	var pre blocked_domain_db.BlockList
	if err := app_db.GetConnection().Where("url = ?", preBlocked).First(&pre).Error; err != nil {
		t.Fatalf("preBlocked vanished from blocklist: %v", err)
	}
	if pre.Source != source_db.SourceHaGeZiMulti.String() {
		t.Errorf("preBlocked Source overwritten: got %q, want %q (Collect must not upgrade existing Source)",
			pre.Source, source_db.SourceHaGeZiMulti)
	}

	// preBlocked — ни в каком виде не появился в suggest_blocks
	// (был отсечён ShouldAutoBlock → autoBlock → already-exists, минуя suggest).
	var preInSuggest int64
	app_db.GetConnection().Model(&suggest_to_block_db.SuggestBlock{}).
		Where("domain = ?", preBlocked).Count(&preInSuggest)
	if preInSuggest != 0 {
		t.Errorf("already-blocked domain leaked into suggest, got %d rows", preInSuggest)
	}

	// suggested — в suggest_blocks, не в blocklist.
	var sugInBlock int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).
		Where("url = ?", suggested).Count(&sugInBlock)
	if sugInBlock != 0 {
		t.Errorf("suggest-only domain leaked into blocklist, got %d rows", sugInBlock)
	}
	var sugRow suggest_to_block_db.SuggestBlock
	if err := app_db.GetConnection().Where("domain = ?", suggested).First(&sugRow).Error; err != nil {
		t.Errorf("suggest-only domain missing from suggest table: %v", err)
	}
}

// TestCollect_Idempotent повторно запускает Collect на тех же данных и
// проверяет, что: (а) количество строк в blocklist не растёт (нет дублей по
// UNIQUE индексу через всплывающие panics), (б) Collect возвращает nil
// (already-exists не считается ошибкой), (в) suggest_blocks тоже не
// дублируются (за это отвечает CreateSuggestBlockBatch dedup, но второй
// запуск — хорошее покрытие).
func TestCollect_Idempotent(t *testing.T) {
	resetTables(t)

	seedBlocked(t, "example.com")
	seedAllowed(t, "x8z7c4kqjfpw9.example.com")
	seedAllowed(t, "x8z7c4kqjfpw9.other.click") // suggest-only

	if err := Collect(); err != nil {
		t.Fatalf("first Collect: %v", err)
	}

	var blockedAfterFirst, suggestAfterFirst int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).Count(&blockedAfterFirst)
	app_db.GetConnection().Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestAfterFirst)

	if err := Collect(); err != nil {
		t.Fatalf("second Collect: %v", err)
	}

	var blockedAfterSecond, suggestAfterSecond int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).Count(&blockedAfterSecond)
	app_db.GetConnection().Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestAfterSecond)

	if blockedAfterSecond != blockedAfterFirst {
		t.Errorf("blocklist grew on second Collect: %d → %d (auto-block must be idempotent)",
			blockedAfterFirst, blockedAfterSecond)
	}
	if suggestAfterSecond != suggestAfterFirst {
		t.Errorf("suggest_blocks grew on second Collect: %d → %d", suggestAfterFirst, suggestAfterSecond)
	}
}

// TestCollect_EmptyInput — happy-path no-op: ни blocked, ни allowed.
// Collect не должен ни паниковать, ни обращаться к
// CreateSuggestBlockBatch со скрытыми побочками (тест ловит, например,
// случай, когда новый код начнёт коммитить пустой батч в транзакции).
func TestCollect_EmptyInput(t *testing.T) {
	resetTables(t)

	if err := Collect(); err != nil {
		t.Fatalf("Collect on empty DB returned error: %v", err)
	}

	var blockedCount, suggestCount int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).Count(&blockedCount)
	app_db.GetConnection().Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestCount)
	if blockedCount != 0 || suggestCount != 0 {
		t.Errorf("expected empty DB after Collect on empty input, got blocked=%d suggest=%d",
			blockedCount, suggestCount)
	}
}

// TestCollect_AllowedButNoSignals — есть allowed-домены, но ни один не
// собирает score >= ThresholdToSuggestBlocking. Collect должен корректно
// отработать, ничего не записать и не вызвать rebuild.
func TestCollect_AllowedButNoSignals(t *testing.T) {
	resetTables(t)

	seedAllowed(t, "plain.example")
	seedAllowed(t, "another-plain.example")

	if err := Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var blockedCount, suggestCount int64
	app_db.GetConnection().Model(&blocked_domain_db.BlockList{}).Count(&blockedCount)
	app_db.GetConnection().Model(&suggest_to_block_db.SuggestBlock{}).Count(&suggestCount)
	if blockedCount != 0 {
		t.Errorf("nothing should be auto-blocked, got %d", blockedCount)
	}
	if suggestCount != 0 {
		t.Errorf("nothing should be suggested, got %d", suggestCount)
	}
}
