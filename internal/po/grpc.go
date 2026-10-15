package po

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ank/e-proc/pkg/grpcx"
	commonv1 "github.com/ank/e-proc/proto/gen/common/v1"
	pov1 "github.com/ank/e-proc/proto/gen/po/v1"
)

// GRPCServer adapts the purchase order service to the generated gRPC contract.
type GRPCServer struct {
	pov1.UnimplementedPurchaseOrderServiceServer
	svc *Service
}

// NewGRPCServer wraps a Service for gRPC transport.
func NewGRPCServer(svc *Service) *GRPCServer { return &GRPCServer{svc: svc} }

// CreatePurchaseOrder implements pov1.PurchaseOrderServiceServer.
func (g *GRPCServer) CreatePurchaseOrder(ctx context.Context, req *pov1.CreatePurchaseOrderRequest) (*pov1.CreatePurchaseOrderResponse, error) {
	in := CreateInput{
		TenantID: req.GetTenantId(),
		TenderID: req.GetTenderId(),
		BidID:    req.GetBidId(),
		VendorID: req.GetVendorId(),
		Notes:    req.GetNotes(),
	}
	for _, line := range req.GetLines() {
		in.Lines = append(in.Lines, Line{
			Name:      line.GetName(),
			Quantity:  line.GetQuantity(),
			UnitPrice: fromProtoMoney(line.GetUnitPrice()),
		})
	}

	order, requiresApproval, err := g.svc.Create(ctx, in, req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &pov1.CreatePurchaseOrderResponse{PurchaseOrder: toProto(order), RequiresApproval: requiresApproval}, nil
}

// GetPurchaseOrder implements pov1.PurchaseOrderServiceServer.
func (g *GRPCServer) GetPurchaseOrder(ctx context.Context, req *pov1.GetPurchaseOrderRequest) (*pov1.GetPurchaseOrderResponse, error) {
	order, err := g.svc.Get(ctx, req.GetTenantId(), req.GetId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &pov1.GetPurchaseOrderResponse{PurchaseOrder: toProto(order)}, nil
}

// UpdatePurchaseOrderStatus implements pov1.PurchaseOrderServiceServer.
func (g *GRPCServer) UpdatePurchaseOrderStatus(ctx context.Context, req *pov1.UpdatePurchaseOrderStatusRequest) (*pov1.UpdatePurchaseOrderStatusResponse, error) {
	order, err := g.svc.UpdateStatus(ctx, req.GetTenantId(), req.GetId(), fromProtoStatus(req.GetStatus()), req.GetActorId(), req.GetApprovalId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &pov1.UpdatePurchaseOrderStatusResponse{PurchaseOrder: toProto(order)}, nil
}

// ListPurchaseOrders implements pov1.PurchaseOrderServiceServer.
func (g *GRPCServer) ListPurchaseOrders(ctx context.Context, req *pov1.ListPurchaseOrdersRequest) (*pov1.ListPurchaseOrdersResponse, error) {
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

	orders, total, err := g.svc.List(ctx, ListFilter{
		TenantID: req.GetTenantId(),
		VendorID: req.GetVendorId(),
		Status:   fromProtoStatus(req.GetStatus()),
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		return nil, grpcx.Error(err)
	}

	out := make([]*pov1.PurchaseOrder, 0, len(orders))
	for i := range orders {
		out = append(out, toProto(&orders[i]))
	}
	return &pov1.ListPurchaseOrdersResponse{
		PurchaseOrders: out,
		Page:           &commonv1.PageResponse{Page: int32(page), PerPage: int32(perPage), Total: total},
	}, nil
}

// ClosePurchaseOrder implements pov1.PurchaseOrderServiceServer.
func (g *GRPCServer) ClosePurchaseOrder(ctx context.Context, req *pov1.ClosePurchaseOrderRequest) (*pov1.ClosePurchaseOrderResponse, error) {
	order, err := g.svc.Close(ctx, req.GetTenantId(), req.GetId(), req.GetActorId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &pov1.ClosePurchaseOrderResponse{PurchaseOrder: toProto(order)}, nil
}

func toProto(p *PurchaseOrder) *pov1.PurchaseOrder {
	if p == nil {
		return nil
	}
	lines := make([]*pov1.PurchaseOrderLine, 0, len(p.Lines))
	for _, line := range p.Lines {
		lines = append(lines, &pov1.PurchaseOrderLine{
			Name:       line.Name,
			Quantity:   line.Quantity,
			UnitPrice:  &commonv1.Money{Amount: line.UnitPrice.Amount, Currency: line.UnitPrice.Currency},
			TotalPrice: &commonv1.Money{Amount: line.TotalPrice.Amount, Currency: line.TotalPrice.Currency},
		})
	}
	order := &pov1.PurchaseOrder{
		Id:         p.ID,
		TenantId:   p.TenantID,
		Number:     p.Number,
		TenderId:   p.TenderID,
		BidId:      p.BidID,
		VendorId:   p.VendorID,
		Lines:      lines,
		Total:      &commonv1.Money{Amount: p.Total.Amount, Currency: p.Total.Currency},
		Status:     toProtoStatus(p.Status),
		ApprovalId: p.ApprovalID,
		CreatedAt:  timestamppb.New(p.CreatedAt),
		UpdatedAt:  timestamppb.New(p.UpdatedAt),
		CreatedBy:  p.CreatedBy,
		Notes:      p.Notes,
	}
	return order
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

func toProtoStatus(s Status) pov1.PurchaseOrderStatus {
	switch s {
	case StatusDraft:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_DRAFT
	case StatusPendingApproval:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_PENDING_APPROVAL
	case StatusApproved:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_APPROVED
	case StatusRejected:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_REJECTED
	case StatusIssued:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_ISSUED
	case StatusClosed:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_CLOSED
	default:
		return pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_UNSPECIFIED
	}
}

func fromProtoStatus(s pov1.PurchaseOrderStatus) Status {
	switch s {
	case pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_DRAFT:
		return StatusDraft
	case pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_PENDING_APPROVAL:
		return StatusPendingApproval
	case pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_APPROVED:
		return StatusApproved
	case pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_REJECTED:
		return StatusRejected
	case pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_ISSUED:
		return StatusIssued
	case pov1.PurchaseOrderStatus_PURCHASE_ORDER_STATUS_CLOSED:
		return StatusClosed
	default:
		return ""
	}
}
