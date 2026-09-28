package payment

import (
	"context"
	"time"

	"online_assesment_edu/internal/ids"
)

type ChargeInput struct {
	BookingID      string
	AmountCents    int
	IdempotencyKey string
}

type ChargeResult struct {
	Result    string
	Reference string
}

type Gateway interface {
	Charge(ctx context.Context, in ChargeInput) (ChargeResult, error)
}

// Scripted honors the demo outcome field. Tests can inject Delay.
type Scripted struct {
	Delay time.Duration
}

func (g Scripted) Charge(ctx context.Context, in ChargeInput) (ChargeResult, error) {
	if g.Delay > 0 {
		select {
		case <-ctx.Done():
			return ChargeResult{}, ctx.Err()
		case <-time.After(g.Delay):
		}
	}
	return ChargeResult{
		Result:    "success",
		Reference: ids.New("payref"),
	}, nil
}

type Fixed struct {
	Result string
	Delay  time.Duration
}

func (g Fixed) Charge(ctx context.Context, in ChargeInput) (ChargeResult, error) {
	if g.Delay > 0 {
		select {
		case <-ctx.Done():
			return ChargeResult{}, ctx.Err()
		case <-time.After(g.Delay):
		}
	}
	result := g.Result
	if result == "" {
		result = "success"
	}
	return ChargeResult{Result: result, Reference: ids.New("payref")}, nil
}
