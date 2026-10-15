package bid

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
)

// cacheTTL is how long a single bid document is cached.
const cacheTTL = 5 * time.Minute

// Service applies bid business rules on top of the repository.
type Service struct {
	repo      *Repository
	cache     *cache.Client
	publisher *broker.Publisher
	log       *slog.Logger
}

// NewService builds the bid service.
func NewService(repo *Repository, cacheClient *cache.Client, publisher *broker.Publisher, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cacheClient, publisher: publisher, log: log}
}

// EnsureIndexes is called once at boot.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.repo.EnsureIndexes(ctx) }

// CacheKey is the Redis key for one bid document.
func CacheKey(id string) string { return "cache:bid:" + id }

// Submit validates a quotation, prices it server-side and stores it.
func (s *Service) Submit(ctx context.Context, in SubmitInput, actorID string) (*Bid, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		return nil, apperr.NewValidation(errs)
	}

	items, total := in.PriceItems()

	b := &Bid{
		TenantID:     in.TenantID,
		TenderID:     in.TenderID,
		VendorID:     in.VendorID,
		Items:        items,
		Total:        total,
		DeliveryDays: in.DeliveryDays,
		Notes:        in.Notes,
		Status:       StatusSubmitted,
		SubmittedBy:  actorID,
	}
	if err := s.repo.Create(ctx, b); err != nil {
		return nil, err
	}

	s.emit(ctx, broker.EventBidSubmitted, b.TenantID, actorID, map[string]any{
		"bid_id":        b.ID,
		"tender_id":     b.TenderID,
		"vendor_id":     b.VendorID,
		"total":         b.Total,
		"delivery_days": b.DeliveryDays,
	})
	return b, nil
}

// Get returns one bid, serving it from cache when possible.
func (s *Service) Get(ctx context.Context, tenantID, id string) (*Bid, error) {
	var cached Bid
	if err := s.cache.GetJSON(ctx, CacheKey(id), &cached); err == nil && cached.TenantID == tenantID {
		return &cached, nil
	}

	b, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetJSON(ctx, CacheKey(b.ID), b, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "cache write failed", slog.String("error", err.Error()))
	}
	return b, nil
}

// List returns a page of bids.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Bid, int64, error) {
	if f.Status != "" && !IsValidStatus(f.Status) {
		return nil, 0, apperr.NewValidation(map[string]string{"status": "unknown bid status"})
	}
	return s.repo.List(ctx, f)
}

// Compare builds the price comparison report for a tender: every non-rejected
// bid ranked by total, with the cheapest and the fastest flagged.
func (s *Service) Compare(ctx context.Context, tenantID, tenderID string) ([]Comparison, Money, error) {
	if tenderID == "" {
		return nil, Money{}, apperr.NewValidation(map[string]string{"tender_id": "tender_id is required"})
	}

	bids, err := s.repo.ListForComparison(ctx, tenantID, tenderID)
	if err != nil {
		return nil, Money{}, err
	}
	if len(bids) == 0 {
		return []Comparison{}, Money{}, nil
	}

	lowest := Money{Amount: math.MaxInt64, Currency: bids[0].Total.Currency}
	fastest := math.MaxInt32
	for i := range bids {
		if bids[i].Total.Amount < lowest.Amount {
			lowest = bids[i].Total
		}
		if bids[i].DeliveryDays < fastest {
			fastest = bids[i].DeliveryDays
		}
	}

	comparisons := make([]Comparison, 0, len(bids))
	for i := range bids {
		b := bids[i]
		delta := 0.0
		if lowest.Amount > 0 {
			delta = (float64(b.Total.Amount) - float64(lowest.Amount)) / float64(lowest.Amount) * 100
		}
		comparisons = append(comparisons, Comparison{
			BidID:           b.ID,
			VendorID:        b.VendorID,
			Total:           b.Total,
			DeliveryDays:    b.DeliveryDays,
			Status:          b.Status,
			LowestPrice:     b.Total.Amount == lowest.Amount,
			FastestDelivery: b.DeliveryDays == fastest,
			Rank:            i + 1,
			PriceDeltaPct:   math.Round(delta*100) / 100,
		})
	}
	return comparisons, lowest, nil
}

// Select marks one bid as selected and rejects the rest of that tender's bids.
func (s *Service) Select(ctx context.Context, tenantID, bidID, actorID string) (*Bid, error) {
	current, err := s.repo.GetByID(ctx, tenantID, bidID)
	if err != nil {
		return nil, err
	}
	if current.Status == StatusSelected {
		return current, nil
	}
	if current.Status == StatusRejected {
		return nil, apperr.NewConflict("bid %s is already rejected", current.ID)
	}

	selected, err := s.repo.SetStatus(ctx, tenantID, bidID, StatusSelected)
	if err != nil {
		return nil, err
	}
	rejected, err := s.repo.RejectOthers(ctx, tenantID, current.TenderID, bidID)
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, selected.ID)

	s.emit(ctx, broker.EventBidSelected, selected.TenantID, actorID, map[string]any{
		"bid_id":        selected.ID,
		"tender_id":     selected.TenderID,
		"vendor_id":     selected.VendorID,
		"total":         selected.Total,
		"rejected_bids": rejected,
	})
	return selected, nil
}

func (s *Service) invalidate(ctx context.Context, id string) {
	if err := s.cache.Delete(ctx, CacheKey(id)); err != nil {
		s.log.WarnContext(ctx, "cache invalidate failed", slog.String("error", err.Error()))
	}
}

func (s *Service) emit(ctx context.Context, name, tenantID, actorID string, payload any) {
	if s.publisher == nil {
		return
	}
	evt, err := broker.NewEvent(name, tenantID, actorID, payload)
	if err != nil {
		s.log.ErrorContext(ctx, "build event failed", slog.String("event", name), slog.String("error", err.Error()))
		return
	}
	if err := s.publisher.Publish(ctx, evt); err != nil && !errors.Is(err, context.Canceled) {
		s.log.ErrorContext(ctx, "publish event failed", slog.String("event", name), slog.String("error", err.Error()))
	}
}
