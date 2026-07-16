package payments

import "context"

type Service interface {
	ListPayments(ctx context.Context) ([]Payment, error)
}

type svc struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &svc{repo: repo}
}

func (s *svc) ListPayments(ctx context.Context) ([]Payment, error) {
	return s.repo.ListPayments(ctx)
}
