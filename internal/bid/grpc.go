package bid

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ank/e-proc/pkg/grpcx"
	bidv1 "github.com/ank/e-proc/proto/gen/bid/v1"
	commonv1 "github.com/ank/e-proc/proto/gen/common/v1"
)

// GRPCServer adapts the bid service to the generated gRPC contract.
type GRPCServer struct {
	bidv1.UnimplementedBidServiceServer
	svc *Service
}

// NewGRPCServer wraps a Service for gRPC transport.
func NewGRPCServer(svc *Service) *GRPCServer { return &GRPCServer{svc: svc} }

// SubmitBid implements bidv1.BidServiceServer.
func (g *GRPCServer) SubmitBid(ctx context.Context, req *bidv1.SubmitBidRequest) (*bidv1.SubmitBidResponse, error) {
	in := SubmitInput{
		TenantID:     req.GetTenantId(),
		TenderID:     req.GetTenderId(),
		VendorID:     req.GetVendorId(),
		DeliveryDays: int(req.GetDeliveryDays()),
		Notes:        req.GetNotes(),
	}
	for _, item := range req.GetItems() {
		in.Items = append(in.Items, Item{
			Name:      item.GetName(),
			Quantity:  item.GetQuantity(),
			UnitPrice: fromProtoMoney(item.GetUnitPrice()),
		})
	}

	b, err := g.svc.Submit(ctx, in, req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &bidv1.SubmitBidResponse{Bid: toProto(b)}, nil
}

// GetBid implements bidv1.BidServiceServer.
func (g *GRPCServer) GetBid(ctx context.Context, req *bidv1.GetBidRequest) (*bidv1.GetBidResponse, error) {
	b, err := g.svc.Get(ctx, req.GetTenantId(), req.GetId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &bidv1.GetBidResponse{Bid: toProto(b)}, nil
}

// ListBidsByTender implements bidv1.BidServiceServer.
func (g *GRPCServer) ListBidsByTender(ctx context.Context, req *bidv1.ListBidsByTenderRequest) (*bidv1.ListBidsByTenderResponse, error) {
	page, perPage := 1, 20
	if req.GetPage() != nil {
		page = int(req.GetPage().GetPage())
		perPage = int(req.GetPage().GetPerPage())
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	bids, total, err := g.svc.List(ctx, ListFilter{
		TenantID: req.GetTenantId(),
		TenderID: req.GetTenderId(),
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		return nil, grpcx.Error(err)
	}

	out := make([]*bidv1.Bid, 0, len(bids))
	for i := range bids {
		out = append(out, toProto(&bids[i]))
	}
	return &bidv1.ListBidsByTenderResponse{
		Bids: out,
		Page: &commonv1.PageResponse{Page: int32(page), PerPage: int32(perPage), Total: total},
	}, nil
}

// CompareBids implements bidv1.BidServiceServer.
func (g *GRPCServer) CompareBids(ctx context.Context, req *bidv1.CompareBidsRequest) (*bidv1.CompareBidsResponse, error) {
	comparisons, lowest, err := g.svc.Compare(ctx, req.GetTenantId(), req.GetTenderId())
	if err != nil {
		return nil, grpcx.Error(err)
	}

	out := make([]*bidv1.BidComparison, 0, len(comparisons))
	for _, c := range comparisons {
		out = append(out, &bidv1.BidComparison{
			BidId:           c.BidID,
			VendorId:        c.VendorID,
			Total:           &commonv1.Money{Amount: c.Total.Amount, Currency: c.Total.Currency},
			DeliveryDays:    int32(c.DeliveryDays),
			LowestPrice:     c.LowestPrice,
			FastestDelivery: c.FastestDelivery,
		})
	}
	return &bidv1.CompareBidsResponse{
		Comparisons: out,
		LowestTotal: &commonv1.Money{Amount: lowest.Amount, Currency: lowest.Currency},
	}, nil
}

// SelectBid implements bidv1.BidServiceServer.
func (g *GRPCServer) SelectBid(ctx context.Context, req *bidv1.SelectBidRequest) (*bidv1.SelectBidResponse, error) {
	b, err := g.svc.Select(ctx, req.GetTenantId(), req.GetBidId(), req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &bidv1.SelectBidResponse{Bid: toProto(b)}, nil
}

func toProto(b *Bid) *bidv1.Bid {
	if b == nil {
		return nil
	}
	items := make([]*bidv1.BidItem, 0, len(b.Items))
	for _, item := range b.Items {
		items = append(items, &bidv1.BidItem{
			Name:       item.Name,
			Quantity:   item.Quantity,
			UnitPrice:  &commonv1.Money{Amount: item.UnitPrice.Amount, Currency: item.UnitPrice.Currency},
			TotalPrice: &commonv1.Money{Amount: item.TotalPrice.Amount, Currency: item.TotalPrice.Currency},
		})
	}
	return &bidv1.Bid{
		Id:           b.ID,
		TenantId:     b.TenantID,
		TenderId:     b.TenderID,
		VendorId:     b.VendorID,
		Items:        items,
		Total:        &commonv1.Money{Amount: b.Total.Amount, Currency: b.Total.Currency},
		DeliveryDays: int32(b.DeliveryDays),
		Notes:        b.Notes,
		Status:       toProtoStatus(b.Status),
		SubmittedAt:  timestamppb.New(b.SubmittedAt),
		UpdatedAt:    timestamppb.New(b.UpdatedAt),
		SubmittedBy:  b.SubmittedBy,
	}
}

func fromProtoMoney(m *commonv1.Money) Money {
	if m == nil {
		return Money{Currency: "IDR"}
	}
	currency := m.GetCurrency()
	if currency == "" {
		currency = "IDR"
	}
	return Money{Amount: m.GetAmount(), Currency: currency}
}

func toProtoStatus(s Status) bidv1.BidStatus {
	switch s {
	case StatusSubmitted:
		return bidv1.BidStatus_BID_STATUS_SUBMITTED
	case StatusShortlisted:
		return bidv1.BidStatus_BID_STATUS_SHORTLISTED
	case StatusSelected:
		return bidv1.BidStatus_BID_STATUS_SELECTED
	case StatusRejected:
		return bidv1.BidStatus_BID_STATUS_REJECTED
	default:
		return bidv1.BidStatus_BID_STATUS_UNSPECIFIED
	}
}
