package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"

	"online_assesment_edu/internal/ids"
	"online_assesment_edu/internal/payment"
)

func isMySQLNumber(err error, number uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == number
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	var last error
	for i := 0; i < 4; i++ {
		tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return err
		}
		last = fn(tx)
		if last == nil {
			if err := tx.Commit(); err != nil {
				if isMySQLNumber(err, 1213) {
					time.Sleep(time.Duration(i+1) * 15 * time.Millisecond)
					continue
				}
				return err
			}
			return nil
		}
		_ = tx.Rollback()
		if isMySQLNumber(last, 1213) {
			time.Sleep(time.Duration(i+1) * 15 * time.Millisecond)
			continue
		}
		return last
	}
	return last
}

func (s *Store) now() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func (s *Store) expireHolds(ctx context.Context, tx *sql.Tx, classID string, now time.Time) error {
	qBook := `UPDATE bookings
SET status = ?, updated_at = ?
WHERE status = ? AND hold_expires_at <= ?`
	qSeat := `UPDATE seats
SET status = ?, booking_id = NULL, hold_expires_at = NULL
WHERE status = ? AND hold_expires_at <= ?`
	argsBook := []any{StatusExpired, now, StatusPending, now}
	argsSeat := []any{SeatFree, SeatHeld, now}
	if classID != "" {
		qBook += ` AND trial_class_id = ?`
		qSeat += ` AND trial_class_id = ?`
		argsBook = append(argsBook, classID)
		argsSeat = append(argsSeat, classID)
	}
	if _, err := tx.ExecContext(ctx, qBook, argsBook...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, qSeat, argsSeat...); err != nil {
		return err
	}
	return nil
}

func (s *Store) ParentExists(ctx context.Context, parentID string) (bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM parents WHERE id = ?`, parentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ClassExists(ctx context.Context, classID string) (bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM trial_classes WHERE id = ?`, classID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListStudents(ctx context.Context, parentID string) ([]Student, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, parent_id, name, age FROM students WHERE parent_id = ? ORDER BY name`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Student
	for rows.Next() {
		var st Student
		if err := rows.Scan(&st.ID, &st.ParentID, &st.Name, &st.Age); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) ListClasses(ctx context.Context) ([]TrialClass, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		return s.expireHolds(ctx, tx, "", s.now())
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT
  c.id, c.subject, c.title, c.starts_at, c.teacher_name, c.capacity, c.amount_cents,
  COALESCE(SUM(s.status = 'confirmed'), 0),
  COALESCE(SUM(s.status = 'held'), 0),
  COALESCE(SUM(s.status = 'free'), 0)
FROM trial_classes c
JOIN seats s ON s.trial_class_id = c.id
GROUP BY c.id, c.subject, c.title, c.starts_at, c.teacher_name, c.capacity, c.amount_cents
ORDER BY c.starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrialClass
	for rows.Next() {
		var c TrialClass
		if err := rows.Scan(&c.ID, &c.Subject, &c.Title, &c.StartsAt, &c.TeacherName, &c.Capacity, &c.AmountCents,
			&c.ConfirmedCount, &c.HeldCount, &c.RemainingSeats); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetRoster(ctx context.Context, classID string) (*Roster, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		return s.expireHolds(ctx, tx, classID, s.now())
	})
	if err != nil {
		return nil, err
	}
	var r Roster
	err = s.DB.QueryRowContext(ctx, `
SELECT
  c.id, c.title, c.capacity,
  COALESCE(SUM(s.status = 'confirmed'), 0),
  COALESCE(SUM(s.status = 'held'), 0),
  COALESCE(SUM(s.status = 'free'), 0)
FROM trial_classes c
JOIN seats s ON s.trial_class_id = c.id
WHERE c.id = ?
GROUP BY c.id, c.title, c.capacity`, classID).Scan(
		&r.TrialClassID, &r.Title, &r.Capacity, &r.ConfirmedCount, &r.HeldCount, &r.RemainingSeats)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT b.id, st.id, st.name, p.name, b.confirmed_at
FROM bookings b
JOIN students st ON st.id = b.student_id
JOIN parents p ON p.id = b.parent_id
WHERE b.trial_class_id = ? AND b.status = ?
ORDER BY b.confirmed_at`, classID, StatusConfirmed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r.Students = []RosterEntry{}
	for rows.Next() {
		var e RosterEntry
		if err := rows.Scan(&e.BookingID, &e.StudentID, &e.StudentName, &e.ParentName, &e.ConfirmedAt); err != nil {
			return nil, err
		}
		r.Students = append(r.Students, e)
	}
	return &r, rows.Err()
}

func (s *Store) GetBooking(ctx context.Context, bookingID, parentID string) (*Booking, error) {
	b, err := s.getBooking(ctx, s.DB, bookingID)
	if err != nil {
		return nil, err
	}
	if b.ParentID != parentID {
		return nil, ErrForbiddenStudent
	}
	return b, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) getBooking(ctx context.Context, q queryer, bookingID string) (*Booking, error) {
	var b Booking
	err := q.QueryRowContext(ctx, `
SELECT
  b.id, b.parent_id, b.student_id, b.trial_class_id, b.seat_id, b.status,
  b.hold_expires_at, b.confirmed_at, b.amount_cents, b.refund_needed,
  b.created_at, b.updated_at,
  (
    SELECT pa.result FROM payment_attempts pa
    WHERE pa.booking_id = b.id
    ORDER BY pa.created_at DESC
    LIMIT 1
  ),
  st.name, p.name, c.title, c.subject, c.teacher_name, c.starts_at
FROM bookings b
JOIN students st ON st.id = b.student_id
JOIN parents p ON p.id = b.parent_id
JOIN trial_classes c ON c.id = b.trial_class_id
WHERE b.id = ?`, bookingID).Scan(
		&b.ID, &b.ParentID, &b.StudentID, &b.TrialClassID, &b.SeatID, &b.Status,
		&b.HoldExpiresAt, &b.ConfirmedAt, &b.AmountCents, &b.RefundNeeded,
		&b.CreatedAt, &b.UpdatedAt, &b.LastPaymentResult,
		&b.StudentName, &b.ParentName, &b.ClassTitle, &b.ClassSubject, &b.TeacherName, &b.ClassStartsAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) studentOwned(ctx context.Context, tx *sql.Tx, parentID, studentID string) error {
	var owner string
	err := tx.QueryRowContext(ctx, `SELECT parent_id FROM students WHERE id = ?`, studentID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner != parentID {
		return ErrForbiddenStudent
	}
	return nil
}

type CreateResult struct {
	Booking *Booking
	Created bool
}

func (s *Store) CreateBooking(ctx context.Context, parentID, studentID, classID string) (*CreateResult, error) {
	if parentID == "" || studentID == "" || classID == "" {
		return nil, ErrValidation
	}
	var out *CreateResult
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.now()
		if err := s.expireHolds(ctx, tx, classID, now); err != nil {
			return err
		}
		if err := s.studentOwned(ctx, tx, parentID, studentID); err != nil {
			return err
		}
		var amount int
		err := tx.QueryRowContext(ctx, `
SELECT amount_cents FROM trial_classes WHERE id = ?`, classID).Scan(&amount)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		var existingID, existingStatus string
		err = tx.QueryRowContext(ctx, `
SELECT id, status FROM bookings
WHERE student_id = ? AND trial_class_id = ?
  AND status IN (?, ?)
LIMIT 1
FOR UPDATE`, studentID, classID, StatusConfirmed, StatusPending).Scan(&existingID, &existingStatus)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if existingStatus == StatusConfirmed {
				return ErrDuplicateConfirmed
			}
			b, gerr := s.getBooking(ctx, tx, existingID)
			if gerr != nil {
				return gerr
			}
			out = &CreateResult{Booking: b, Created: false}
			return nil
		}

		var seatID string
		err = tx.QueryRowContext(ctx, `
SELECT id FROM seats
WHERE trial_class_id = ? AND status = ?
LIMIT 1
FOR UPDATE SKIP LOCKED`, classID, SeatFree).Scan(&seatID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrClassFull
		}
		if err != nil {
			return err
		}

		bookingID := ids.New("bkg")
		expires := now.Add(s.HoldTTL)
		_, err = tx.ExecContext(ctx, `
INSERT INTO bookings (
  id, parent_id, student_id, trial_class_id, seat_id, status,
  hold_expires_at, confirmed_at, amount_cents, refund_needed, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, 0, ?, ?)`,
			bookingID, parentID, studentID, classID, seatID, StatusPending,
			expires, amount, now, now)
		if isMySQLNumber(err, 1062) {
			// Roll back so the SKIP LOCKED seat is not held unused.
			return errReusePending
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
UPDATE seats
SET status = ?, booking_id = ?, hold_expires_at = ?
WHERE id = ? AND status = ?`, SeatHeld, bookingID, expires, seatID, SeatFree)
		if err != nil {
			return err
		}
		b, gerr := s.getBooking(ctx, tx, bookingID)
		if gerr != nil {
			return gerr
		}
		out = &CreateResult{Booking: b, Created: true}
		return nil
	})
	if errors.Is(err, errReusePending) {
		b, gerr := s.lookupActiveBooking(ctx, studentID, classID)
		if gerr != nil {
			return nil, gerr
		}
		if b != nil && b.Status == StatusPending {
			return &CreateResult{Booking: b, Created: false}, nil
		}
		if b != nil && b.Status == StatusConfirmed {
			return nil, ErrDuplicateConfirmed
		}
		return nil, ErrClassFull
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) lookupActiveBooking(ctx context.Context, studentID, classID string) (*Booking, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `
SELECT id FROM bookings
WHERE student_id = ? AND trial_class_id = ? AND status IN (?, ?)
LIMIT 1`, studentID, classID, StatusConfirmed, StatusPending).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.getBooking(ctx, s.DB, id)
}

type PayInput struct {
	BookingID      string
	ParentID       string
	IdempotencyKey string
	Outcome        string
	Gateway        payment.Gateway
}

func (s *Store) PayForBooking(ctx context.Context, in PayInput) (*Booking, error) {
	if in.IdempotencyKey == "" {
		return nil, ErrValidation
	}
	outcome := in.Outcome
	if outcome != "success" && outcome != "failure" {
		return nil, ErrValidation
	}

	var existingAttemptID string
	err := s.DB.QueryRowContext(ctx, `
SELECT id FROM payment_attempts WHERE idempotency_key = ?`, in.IdempotencyKey).Scan(&existingAttemptID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		return s.GetBooking(ctx, in.BookingID, in.ParentID)
	}

	type holdSnap struct {
		amount int
		ok     bool
	}
	var snap holdSnap
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.now()
		b, err := s.getBooking(ctx, tx, in.BookingID)
		if err != nil {
			return err
		}
		if b.ParentID != in.ParentID {
			return ErrForbiddenStudent
		}
		if b.Status == StatusConfirmed {
			snap.ok = true
			snap.amount = b.AmountCents
			return nil
		}
		if err := s.expireHolds(ctx, tx, b.TrialClassID, now); err != nil {
			return err
		}
		b, err = s.getBooking(ctx, tx, in.BookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusConfirmed {
			snap.ok = true
			snap.amount = b.AmountCents
			return nil
		}
		if b.Status != StatusPending || !b.HoldExpiresAt.Valid || !b.HoldExpiresAt.Time.After(now) {
			return ErrHoldExpired
		}
		var seatStatus string
		if !b.SeatID.Valid {
			return ErrHoldExpired
		}
		if err := tx.QueryRowContext(ctx, `SELECT status FROM seats WHERE id = ?`, b.SeatID.String).Scan(&seatStatus); err != nil {
			return err
		}
		if seatStatus != SeatHeld {
			return ErrHoldExpired
		}
		snap.ok = true
		snap.amount = b.AmountCents
		return nil
	})
	if err != nil {
		return nil, err
	}

	b0, err := s.GetBooking(ctx, in.BookingID, in.ParentID)
	if err != nil {
		return nil, err
	}
	if b0.Status == StatusConfirmed {
		return b0, nil
	}

	gw := in.Gateway
	if gw == nil {
		gw = payment.Scripted{}
	}
	var charge payment.ChargeResult
	if outcome == "failure" {
		charge = payment.ChargeResult{Result: "failure", Reference: ids.New("payref")}
	} else {
		charge, err = gw.Charge(ctx, payment.ChargeInput{
			BookingID:      in.BookingID,
			AmountCents:    snap.amount,
			IdempotencyKey: in.IdempotencyKey,
		})
		if err != nil {
			return nil, err
		}
		if charge.Result == "" {
			charge.Result = "success"
		}
	}

	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.now()
		b, err := s.getBooking(ctx, tx, in.BookingID)
		if err != nil {
			return err
		}
		if b.ParentID != in.ParentID {
			return ErrForbiddenStudent
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO payment_attempts (id, booking_id, idempotency_key, result, amount_cents, provider_reference, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ids.New("pat"), b.ID, in.IdempotencyKey, charge.Result, b.AmountCents, charge.Reference, now); err != nil {
			if isMySQLNumber(err, 1062) {
				return nil
			}
			return err
		}
		if b.Status == StatusConfirmed {
			return nil
		}
		if err := s.expireHolds(ctx, tx, b.TrialClassID, now); err != nil {
			return err
		}
		b, err = s.getBooking(ctx, tx, in.BookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusConfirmed {
			return nil
		}
		if charge.Result != "success" {
			if b.Status == StatusPending {
				_, err = tx.ExecContext(ctx, `UPDATE bookings SET updated_at = ? WHERE id = ?`, now, b.ID)
				return err
			}
			return nil
		}
		if b.Status != StatusPending || !b.HoldExpiresAt.Valid || !b.HoldExpiresAt.Time.After(now) || !b.SeatID.Valid {
			_, err = tx.ExecContext(ctx, `
UPDATE bookings SET status = ?, refund_needed = 1, updated_at = ? WHERE id = ? AND status <> ?`,
				StatusSeatUnavailable, now, b.ID, StatusConfirmed)
			return err
		}
		res, err := tx.ExecContext(ctx, `
UPDATE seats
SET status = ?, hold_expires_at = NULL
WHERE id = ? AND status = ? AND booking_id = ?`,
			SeatConfirmed, b.SeatID.String, SeatHeld, b.ID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			_, err = tx.ExecContext(ctx, `
UPDATE bookings
SET status = ?, confirmed_at = ?, refund_needed = 0, updated_at = ?
WHERE id = ?`, StatusConfirmed, now, now, b.ID)
			return err
		}
		_, err = tx.ExecContext(ctx, `
UPDATE bookings SET status = ?, refund_needed = 1, updated_at = ? WHERE id = ? AND status <> ?`,
			StatusSeatUnavailable, now, b.ID, StatusConfirmed)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetBooking(ctx, in.BookingID, in.ParentID)
}

func (s *Store) CancelBooking(ctx context.Context, bookingID, parentID string) (*Booking, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.now()
		b, err := s.getBooking(ctx, tx, bookingID)
		if err != nil {
			return err
		}
		if b.ParentID != parentID {
			return ErrForbiddenStudent
		}
		if err := s.expireHolds(ctx, tx, b.TrialClassID, now); err != nil {
			return err
		}
		b, err = s.getBooking(ctx, tx, bookingID)
		if err != nil {
			return err
		}
		if b.Status == StatusConfirmed {
			return ErrInvalidState
		}
		if b.Status != StatusPending {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE bookings SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
			StatusCancelled, now, b.ID, StatusPending); err != nil {
			return err
		}
		if b.SeatID.Valid {
			_, err = tx.ExecContext(ctx, `
UPDATE seats SET status = ?, booking_id = NULL, hold_expires_at = NULL
WHERE id = ? AND booking_id = ? AND status = ?`,
				SeatFree, b.SeatID.String, b.ID, SeatHeld)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetBooking(ctx, bookingID, parentID)
}

func (s *Store) Occupancy(ctx context.Context, classID string) (confirmed, held, free int, err error) {
	err = s.DB.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(status = 'confirmed'), 0),
  COALESCE(SUM(status = 'held'), 0),
  COALESCE(SUM(status = 'free'), 0)
FROM seats WHERE trial_class_id = ?`, classID).Scan(&confirmed, &held, &free)
	if err != nil {
		return 0, 0, 0, err
	}
	return confirmed, held, free, nil
}

func (s *Store) CountPaymentAttempts(ctx context.Context, bookingID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM payment_attempts WHERE booking_id = ?`, bookingID).Scan(&n)
	return n, err
}

func (s *Store) SweepExpired(ctx context.Context) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		return s.expireHolds(ctx, tx, "", s.now())
	})
}

func (s *Store) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}
