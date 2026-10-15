package tender

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ank/e-proc/pkg/grpcx"
	commonv1 "github.com/ank/e-proc/proto/gen/common/v1"
	tenderv1 "github.com/ank/e-proc/proto/gen/tender/v1"
)

// GRPCServer adapts the tender service to the generated gRPC contract.
type GRPCServer struct {
	tenderv1.UnimplementedTenderServiceServer
	svc *Service
}

// NewGRPCServer wraps a Service for gRPC transport.
func NewGRPCServer(svc *Service) *GRPCServer { return &GRPCServer{svc: svc} }

// CreateTender implements tenderv1.TenderServiceServer.
func (g *GRPCServer) CreateTender(ctx context.Context, req *tenderv1.CreateTenderRequest) (*tenderv1.CreateTenderResponse, error) {
	in := CreateInput{
		TenantID:         req.GetTenantId(),
		Code:             req.GetCode(),
		Title:            req.GetTitle(),
		Description:      req.GetDescription(),
		Items:            fromProtoItems(req.GetItems()),
		Budget:           fromProtoMoney(req.GetBudget()),
		InvitedVendorIDs: req.GetInvitedVendorIds(),
	}
	if req.GetCloseAt() != nil {
		in.CloseAt = req.GetCloseAt().AsTime()
	}

	t, err := g.svc.Create(ctx, in, req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &tenderv1.CreateTenderResponse{Tender: toProto(t)}, nil
}

// GetTender implements tenderv1.TenderServiceServer.
func (g *GRPCServer) GetTender(ctx context.Context, req *tenderv1.GetTenderRequest) (*tenderv1.GetTenderResponse, error) {
	t, err := g.svc.Get(ctx, req.GetTenantId(), req.GetId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &tenderv1.GetTenderResponse{Tender: toProto(t)}, nil
}

// PublishTender implements tenderv1.TenderServiceServer.
func (g *GRPCServer) PublishTender(ctx context.Context, req *tenderv1.PublishTenderRequest) (*tenderv1.PublishTenderResponse, error) {
	t, err := g.svc.Publish(ctx, req.GetTenantId(), req.GetId(), req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &tenderv1.PublishTenderResponse{Tender: toProto(t)}, nil
}

// CloseTender implements tenderv1.TenderServiceServer.
func (g *GRPCServer) CloseTender(ctx context.Context, req *tenderv1.CloseTenderRequest) (*tenderv1.CloseTenderResponse, error) {
	t, err := g.svc.Close(ctx, req.GetTenantId(), req.GetId(), req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &tenderv1.CloseTenderResponse{Tender: toProto(t)}, nil
}

// ListTenders implements tenderv1.TenderServiceServer.
func (g *GRPCServer) ListTenders(ctx context.Context, req *tenderv1.ListTendersRequest) (*tenderv1.ListTendersResponse, error) {
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

	tenders, total, err := g.svc.List(ctx, ListFilter{
		TenantID: req.GetTenantId(),
		Search:   req.GetSearch(),
		Status:   fromProtoStatus(req.GetStatus()),
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		return nil, grpcx.Error(err)
	}

	out := make([]*tenderv1.Tender, 0, len(tenders))
	for i := range tenders {
		out = append(out, toProto(&tenders[i]))
	}
	return &tenderv1.ListTendersResponse{
		Tenders: out,
		Page:    &commonv1.PageResponse{Page: int32(page), PerPage: int32(perPage), Total: total},
	}, nil
}

func toProto(t *Tender) *tenderv1.Tender {
	if t == nil {
		return nil
	}
	items := make([]*tenderv1.TenderItem, 0, len(t.Items))
	for _, item := range t.Items {
		items = append(items, &tenderv1.TenderItem{
			Name:          item.Name,
			Quantity:      item.Quantity,
			Unit:          item.Unit,
			Specification: item.Specification,
		})
	}
	return &tenderv1.Tender{
		Id:               t.ID,
		TenantId:         t.TenantID,
		Code:             t.Code,
		Title:            t.Title,
		Description:      t.Description,
		Items:            items,
		Budget:           &commonv1.Money{Amount: t.Budget.Amount, Currency: t.Budget.Currency},
		Status:           toProtoStatus(t.Status),
		OpenAt:           timestamppb.New(t.OpenAt),
		CloseAt:          timestamppb.New(t.CloseAt),
		CreatedAt:        timestamppb.New(t.CreatedAt),
		UpdatedAt:        timestamppb.New(t.UpdatedAt),
		CreatedBy:        t.CreatedBy,
		InvitedVendorIds: t.InvitedVendorIDs,
	}
}

func fromProtoItems(items []*tenderv1.TenderItem) []Item {
	out := make([]Item, 0, len(items))
	for _, item := range items {
		out = append(out, Item{
			Name:          item.GetName(),
			Quantity:      item.GetQuantity(),
			Unit:          item.GetUnit(),
			Specification: item.GetSpecification(),
		})
	}
	return out
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

func toProtoStatus(s Status) tenderv1.TenderStatus {
	switch s {
	case StatusDraft:
		return tenderv1.TenderStatus_TENDER_STATUS_DRAFT
	case StatusPublished:
		return tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED
	case StatusClosed:
		return tenderv1.TenderStatus_TENDER_STATUS_CLOSED
	case StatusCancelled:
		return tenderv1.TenderStatus_TENDER_STATUS_CANCELLED
	case StatusAwarded:
		return tenderv1.TenderStatus_TENDER_STATUS_AWARDED
	default:
		return tenderv1.TenderStatus_TENDER_STATUS_UNSPECIFIED
	}
}

func fromProtoStatus(s tenderv1.TenderStatus) Status {
	switch s {
	case tenderv1.TenderStatus_TENDER_STATUS_DRAFT:
		return StatusDraft
	case tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED:
		return StatusPublished
	case tenderv1.TenderStatus_TENDER_STATUS_CLOSED:
		return StatusClosed
	case tenderv1.TenderStatus_TENDER_STATUS_CANCELLED:
		return StatusCancelled
	case tenderv1.TenderStatus_TENDER_STATUS_AWARDED:
		return StatusAwarded
	default:
		return ""
	}
}
