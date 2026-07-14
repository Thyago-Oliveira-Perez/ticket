package payments

import "context"

type Service interface {
	ListPayments(ctx context.Context) (error)
}

type svc struct {
	// repository
}

func NewService() Service {
	return &svc {}
}

func (s *svc) ListPayments (ctx context.Context) error {
	return nil
}