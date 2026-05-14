package suggest_to_block

import (
	"errors"
	"time"

	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	blocked_domain_use_cases_create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"
	source_db "github.com/alextorq/dns-filter/source/db"
	suggest_to_block_use_cases_collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func CreateSuggestBlockBatch(suggests []suggest_to_block_db.SuggestBlock) error {
	return suggest_to_block_db.CreateSuggestBlockBatch(suggests)
}

func GetRecordsByFilter(params suggest_to_block_db.GetAllParams) (*suggest_to_block_db.GetAllResult, error) {
	return suggest_to_block_db.GetAllSuggestBlocks(params)
}

func ChangeActiveStatus(id uint, active bool) error {
	return suggest_to_block_db.UpdateActiveStatus(id, active)
}

func Collect() error {
	l := logger.GetLogger()
	l.Info("Start collecting suggestions to block domains")
	blocked, err := blocked_domain.GetAllActiveFilters()
	if err != nil {
		return err
	}

	allowed, err := allow_domain.GetAllActiveFilters()
	if err != nil {
		return err
	}

	// When the operator has disabled the AutoBlocked source via the UI, Collect
	// must not write anything to block_lists — neither active nor inactive
	// rows. Such candidates fall through to the suggest queue for manual
	// review, mirroring how Sync() skips disabled sources entirely. We fail
	// closed on a DB error: silently auto-blocking when we can't confirm the
	// source is active would defeat the kill-switch.
	autoBlockEnabled, err := source_db.IsActive(source_db.SourceAutoBlocked)
	if err != nil {
		l.Error(err)
		autoBlockEnabled = false
	}

	forBlock := suggest_to_block_use_cases_collect.CollectSuggest(blocked, allowed)
	suggests := make([]suggest_to_block_db.SuggestBlock, 0, len(forBlock))
	var autoBlocked, autoAlreadyBlocked, autoErrors int

	for _, domain := range forBlock {
		if autoBlockEnabled && suggest_to_block_use_cases_collect.ShouldAutoBlock(domain) {
			switch autoBlock(domain) {
			case autoBlockInserted:
				autoBlocked++
			case autoBlockAlreadyExists:
				autoAlreadyBlocked++
			case autoBlockError:
				autoErrors++
			}
			continue
		}

		reasons := make([]suggest_to_block_db.SuggestBlockReason, 0, len(domain.Reasons))
		for _, r := range domain.Reasons {
			reasons = append(reasons, suggest_to_block_db.SuggestBlockReason{
				Code:       r.Code,
				MatchValue: r.Match,
			})
		}
		suggests = append(suggests, suggest_to_block_db.SuggestBlock{
			Domain:  domain.Domain,
			Score:   domain.Score,
			Reasons: reasons,
		})
	}

	err = CreateSuggestBlockBatch(suggests)
	if err != nil {
		return err
	}

	// Bloom filter must be rebuilt exactly once per Collect, after all auto-
	// promotions land in the DB — otherwise auto-blocked domains keep
	// resolving until the next manual mutation triggers a rebuild. We only
	// rebuild on *newly inserted* domains: already-blocked entries (e.g. from
	// repeated Collect runs) don't change the bloom set, and pure errors
	// produced nothing to rebuild for.
	if autoBlocked > 0 {
		if err := filter.UpdateFilterFromDb(); err != nil {
			l.Error(err)
		}
	}

	l.Info(
		"Finished collecting suggestions to block domains;",
		"auto-blocked:", autoBlocked,
		"already-blocked:", autoAlreadyBlocked,
		"auto-errors:", autoErrors,
		"to-suggest:", len(suggests),
	)
	return nil
}

// autoBlockOutcome distinguishes the three terminal states of autoBlock so the
// caller can tally them separately. Plain bool was hiding "already blocked"
// inside "false" together with real errors — operators couldn't tell whether
// the run was a healthy idempotent re-run or a sea of DB failures.
type autoBlockOutcome int

const (
	autoBlockInserted autoBlockOutcome = iota
	autoBlockAlreadyExists
	autoBlockError
)

// autoBlock promotes a single Suggestion straight to the blocklist with
// SourceAutoBlocked. Errors are logged but never propagated — auto-block is
// best-effort and must not break the rest of the Collect batch.
func autoBlock(s suggest_to_block_use_cases_collect.Suggestion) autoBlockOutcome {
	l := logger.GetLogger()

	err := blocked_domain.CreateDomain(blocked_domain_use_cases_create_domain.RequestBody{
		Domain: s.Domain,
		Source: source_db.SourceAutoBlocked.String(),
	})
	if errors.Is(err, blocked_domain_use_cases_create_domain.ErrDomainAlreadyExists) {
		return autoBlockAlreadyExists
	}
	if err != nil {
		l.Error(err)
		return autoBlockError
	}

	codes := make([]string, 0, len(s.Reasons))
	for _, r := range s.Reasons {
		codes = append(codes, r.Code)
	}
	l.Info("Auto-blocked domain from suggest:", s.Domain, "score:", s.Score, "reasons:", codes)
	return autoBlockInserted
}

func StartCollectSuggest() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	var tryCollect = func() {
		l := logger.GetLogger()
		err := Collect()
		if err != nil {
			l.Error(err)
		}
	}

	tryCollect()

	for range ticker.C {
		tryCollect()
	}
}
