// Package suggest_to_block is the composition root for the suggest-to-block
// feature. Module orchestrates Collect (read blocked + allowed lists, score
// candidates, optionally auto-promote, persist the rest) on a 12h ticker.
package suggest_to_block

import (
	"context"
	"errors"
	"time"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

// BlockRepo is the narrow port over the blocklist this module needs. It must
// satisfy create_domain.Repo for the auto-block path (CreateDomain checks
// DomainNotExist + writes), plus expose GetAllActiveURLs for the input set.
type BlockRepo interface {
	GetAllActiveURLs() ([]string, error)
	DomainNotExist(domain string) bool
	CreateDomain(domain, source string) error
}

type AllowRepo interface {
	GetAllActiveFilters() ([]string, error)
}

type SourceGate interface {
	IsActive(name source_db.BlockListSource) (bool, error)
}

// Filter abstracts filter.Module — Collect rebuilds the bloom only after at
// least one auto-block lands.
type Filter interface {
	UpdateFromDb() error
}

type SuggestRepo interface {
	CreateBatch(suggests []suggest_to_block_db.SuggestBlock) error
	GetByFilter(params suggest_to_block_db.GetAllParams) (*suggest_to_block_db.GetAllResult, error)
	UpdateActive(id uint, active bool) error
}

type Logger interface {
	Info(args ...any)
	Error(err error)
}

type Module struct {
	blockRepo  BlockRepo
	allowRepo  AllowRepo
	sourceGate SourceGate
	filter     Filter
	repo       SuggestRepo
	log        Logger
}

func NewModule(
	blockRepo BlockRepo,
	allowRepo AllowRepo,
	sourceGate SourceGate,
	filter Filter,
	repo SuggestRepo,
	log Logger,
) *Module {
	return &Module{
		blockRepo:  blockRepo,
		allowRepo:  allowRepo,
		sourceGate: sourceGate,
		filter:     filter,
		repo:       repo,
		log:        log,
	}
}

// Collect runs one batch: load blocked + allowed lists, score candidates,
// auto-block those that pass either gate (if the AutoBlocked source is
// enabled), persist the rest as suggestions, and rebuild the bloom only when
// at least one auto-block landed.
func (m *Module) Collect() error {
	m.log.Info("Start collecting suggestions to block domains")
	blocked, err := m.blockRepo.GetAllActiveURLs()
	if err != nil {
		return err
	}

	allowed, err := m.allowRepo.GetAllActiveFilters()
	if err != nil {
		return err
	}

	// When the operator has disabled the AutoBlocked source via the UI, Collect
	// must not write anything to block_lists — neither active nor inactive
	// rows. Such candidates fall through to the suggest queue for manual
	// review, mirroring how Sync() skips disabled sources entirely. We fail
	// closed on a DB error: silently auto-blocking when we can't confirm the
	// source is active would defeat the kill-switch.
	autoBlockEnabled, err := m.sourceGate.IsActive(source_db.SourceAutoBlocked)
	if err != nil {
		m.log.Error(err)
		autoBlockEnabled = false
	}

	forBlock := collect.CollectSuggest(blocked, allowed)
	suggests := make([]suggest_to_block_db.SuggestBlock, 0, len(forBlock))
	var autoBlocked, autoAlreadyBlocked, autoErrors int

	for _, domain := range forBlock {
		if autoBlockEnabled && collect.ShouldAutoBlock(domain) {
			switch m.autoBlock(domain) {
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

	if err := m.repo.CreateBatch(suggests); err != nil {
		return err
	}

	// Bloom filter must be rebuilt exactly once per Collect, after all auto-
	// promotions land in the DB — otherwise auto-blocked domains keep
	// resolving until the next manual mutation triggers a rebuild. We only
	// rebuild on *newly inserted* domains: already-blocked entries don't
	// change the bloom set, and pure errors produced nothing to rebuild for.
	if autoBlocked > 0 {
		if err := m.filter.UpdateFromDb(); err != nil {
			m.log.Error(err)
		}
	}

	m.log.Info(
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
func (m *Module) autoBlock(s collect.Suggestion) autoBlockOutcome {
	err := create_domain.CreateDomain(
		create_domain.Deps{Repo: m.blockRepo, Log: m.log},
		create_domain.RequestBody{
			Domain: s.Domain,
			Source: source_db.SourceAutoBlocked.String(),
		},
	)
	if errors.Is(err, create_domain.ErrDomainAlreadyExists) {
		return autoBlockAlreadyExists
	}
	if err != nil {
		m.log.Error(err)
		return autoBlockError
	}

	codes := make([]string, 0, len(s.Reasons))
	for _, r := range s.Reasons {
		codes = append(codes, r.Code)
	}
	m.log.Info("Auto-blocked domain from suggest:", s.Domain, "score:", s.Score, "reasons:", codes)
	return autoBlockInserted
}

// Start runs Collect immediately and then on a 12h ticker until ctx is done.
// Block forever — call from a goroutine. ctx cancellation is the only way to
// stop the loop cleanly; per-tick errors are logged but do not break it.
func (m *Module) Start(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	tryCollect := func() {
		if err := m.Collect(); err != nil {
			m.log.Error(err)
		}
	}

	tryCollect()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tryCollect()
		}
	}
}
