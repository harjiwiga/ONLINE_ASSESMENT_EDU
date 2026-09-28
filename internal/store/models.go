package store

import (
	"database/sql"
	"time"
)

const (
	StatusPending         = "pending_payment"
	StatusConfirmed       = "confirmed"
	StatusExpired         = "expired"
	StatusCancelled       = "cancelled"
	StatusSeatUnavailable = "seat_unavailable"

	SeatFree      = "free"
	SeatHeld      = "held"
	SeatConfirmed = "confirmed"
)

type Student struct {
	ID       string
	ParentID string
	Name     string
	Age      sql.NullInt64
}

type TrialClass struct {
	ID             string
	Subject        string
	Title          string
	StartsAt       time.Time
	TeacherName    string
	Capacity       int
	AmountCents    int
	ConfirmedCount int
	HeldCount      int
	RemainingSeats int
}

type Booking struct {
	ID                string
	ParentID          string
	StudentID         string
	TrialClassID      string
	SeatID            sql.NullString
	Status            string
	HoldExpiresAt     sql.NullTime
	ConfirmedAt       sql.NullTime
	AmountCents       int
	RefundNeeded      bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastPaymentResult sql.NullString
	StudentName       string
	ParentName        string
	ClassTitle        string
	ClassSubject      string
	TeacherName       string
	ClassStartsAt     time.Time
}

type RosterEntry struct {
	BookingID   string
	StudentID   string
	StudentName string
	ParentName  string
	ConfirmedAt time.Time
}

type Roster struct {
	TrialClassID   string
	Title          string
	Capacity       int
	ConfirmedCount int
	HeldCount      int
	RemainingSeats int
	Students       []RosterEntry
}

type PaymentAttempt struct {
	ID                string
	BookingID         string
	IdempotencyKey    string
	Result            string
	AmountCents       int
	ProviderReference sql.NullString
	CreatedAt         time.Time
}
