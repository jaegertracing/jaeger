package customer

import "context"

// Customer contains data about a customer.
type Customer struct {
	ID       string
	Name     string
	Location string
}

// Interface exposed by the Customer service.
type Interface interface {
	Get(ctx context.Context, customerID string) (*Customer, error)
}
