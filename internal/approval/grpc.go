package approval

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/grpcx"
	approvalv1 "github.com/ank/e-proc/proto/gen/approval/v1"
	commonv1 "github.com/ank/e-proc/proto/gen/common/v1"
)

// GRPCServer adapts the approval service to the generated gRPC contract.
type GRPCServer struct {
	approvalv1.UnimplementedApprovalServiceServer
	svc *Service
}

// NewGRPCServer wraps a Service for gRPC transport.
func NewGRPCServer(svc *Service) *GRPCServer { return &GRPCServer{svc: svc} }

// RequestApproval implements approvalv1.ApprovalServiceServer.
func (g *GRPCServer) RequestApproval(ctx context.Context, req *approvalv1.RequestApprovalRequest) (*approvalv1.RequestApprovalResponse, error) {
	a, err := g.svc.Request(ctx, RequestInput{
		TenantID:   req.GetTenantId(),
		EntityType: req.GetEntityType(),
		EntityID:   req.GetEntityId(),
		Amount:     req.GetAmount().GetAmount(),
		Currency:   req.GetAmount().GetCurrency(),
		AssignedTo: req.GetAssignedTo(),
	}, req.GetRequestedBy())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &approvalv1.RequestApprovalResponse{Approval: toProto(a)}, nil
}

// DecideApproval implements approvalv1.ApprovalServiceServer.
func (g *GRPCServer) DecideApproval(ctx context.Context, req *approvalv1.DecideApprovalRequest) (*approvalv1.DecideApprovalResponse, error) {
	claims := &auth.Claims{UserID: req.GetActorId(), Role: auth.RoleApprover}
	a, err := g.svc.Decide(ctx, req.GetTenantId(), req.GetId(), DecideInput{
		Approve: req.GetApprove(),
		Comment: req.GetComment(),
	}, claims)
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &approvalv1.DecideApprovalResponse{Approval: toProto(a)}, nil
}

// GetApproval implements approvalv1.ApprovalServiceServer.
func (g *GRPCServer) GetApproval(ctx context.Context, req *approvalv1.GetApprovalRequest) (*approvalv1.GetApprovalResponse, error) {
	a, err := g.svc.Get(ctx, req.GetTenantId(), req.GetId())
	if err != nil {
		return nil, grpcx.Error(err)
	}
	return &approvalv1.GetApprovalResponse{Approval: toProto(a)}, nil
}

// ListApprovals implements approvalv1.ApprovalServiceServer.
func (g *GRPCServer) ListApprovals(ctx context.Context, req *approvalv1.ListApprovalsRequest) (*approvalv1.ListApprovalsResponse, error) {
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

	approvals, total, err := g.svc.List(ctx, ListFilter{
		TenantID:   req.GetTenantId(),
		Status:     fromProtoStatus(req.GetStatus()),
		AssignedTo: req.GetAssignedTo(),
		Page:       page,
		PerPage:    perPage,
	})
	if err != nil {
		return nil, grpcx.Error(err)
	}

	out := make([]*approvalv1.Approval, 0, len(approvals))
	for i := range approvals {
		out = append(out, toProto(&approvals[i]))
	}
	return &approvalv1.ListApprovalsResponse{
		Approvals: out,
		Page:      &commonv1.PageResponse{Page: int32(page), PerPage: int32(perPage), Total: total},
	}, nil
}

func toProto(a *Approval) *approvalv1.Approval {
	if a == nil {
		return nil
	}
	history := make([]*approvalv1.ApprovalStep, 0, len(a.History))
	for _, step := range a.History {
		history = append(history, &approvalv1.ApprovalStep{
			ActorId:   step.ActorID,
			Decision:  toProtoStatus(step.Decision),
			Comment:   step.Comment,
			DecidedAt: timestamppb.New(step.DecidedAt),
		})
	}
	return &approvalv1.Approval{
		Id:          a.ID,
		TenantId:    a.TenantID,
		EntityType:  string(a.EntityType),
		EntityId:    a.EntityID,
		Amount:      &commonv1.Money{Amount: a.Amount.Amount, Currency: a.Amount.Currency},
		Status:      toProtoStatus(a.Status),
		RequestedBy: a.RequestedBy,
		AssignedTo:  a.AssignedTo,
		History:     history,
		CreatedAt:   timestamppb.New(a.CreatedAt),
		UpdatedAt:   timestamppb.New(a.UpdatedAt),
		Comment:     a.Comment,
	}
}

func toProtoStatus(s Status) approvalv1.ApprovalStatus {
	switch s {
	case StatusPending:
		return approvalv1.ApprovalStatus_APPROVAL_STATUS_PENDING
	case StatusApproved:
		return approvalv1.ApprovalStatus_APPROVAL_STATUS_APPROVED
	case StatusRejected:
		return approvalv1.ApprovalStatus_APPROVAL_STATUS_REJECTED
	case StatusCancelled:
		return approvalv1.ApprovalStatus_APPROVAL_STATUS_CANCELLED
	default:
		return approvalv1.ApprovalStatus_APPROVAL_STATUS_UNSPECIFIED
	}
}

func fromProtoStatus(s approvalv1.ApprovalStatus) Status {
	switch s {
	case approvalv1.ApprovalStatus_APPROVAL_STATUS_PENDING:
		return StatusPending
	case approvalv1.ApprovalStatus_APPROVAL_STATUS_APPROVED:
		return StatusApproved
	case approvalv1.ApprovalStatus_APPROVAL_STATUS_REJECTED:
		return StatusRejected
	case approvalv1.ApprovalStatus_APPROVAL_STATUS_CANCELLED:
		return StatusCancelled
	default:
		return ""
	}
}
