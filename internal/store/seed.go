package store

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) Seed(ctx context.Context) error {
	now := s.now()
	mathStart := time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC)
	sciStart := time.Date(2026, 10, 4, 4, 0, 0, 0, time.UTC)

	parents := [][3]string{
		{"par_maya", "Maya", "maya@example.com"},
		{"par_ben", "Ben", "ben@example.com"},
		{"par_chen", "Chen", "chen@example.com"},
		{"par_dana", "Dana", "dana@example.com"},
		{"par_priya", "Priya", "priya@example.com"},
		{"par_omar", "Omar", "omar@example.com"},
	}
	for _, p := range parents {
		if _, err := s.DB.ExecContext(ctx, `
INSERT INTO parents (id, name, email) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email)`, p[0], p[1], p[2]); err != nil {
			return err
		}
	}

	students := []struct {
		ID, Parent, Name string
		Age              int
	}{
		{"stu_aisha", "par_maya", "Aisha", 8},
		{"stu_noah", "par_ben", "Noah", 9},
		{"stu_li", "par_chen", "Li", 8},
		{"stu_sam", "par_dana", "Sam", 10},
		{"stu_arjun", "par_priya", "Arjun", 9},
		{"stu_yasmin", "par_omar", "Yasmin", 8},
	}
	for _, st := range students {
		if _, err := s.DB.ExecContext(ctx, `
INSERT INTO students (id, parent_id, name, age) VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE parent_id = VALUES(parent_id), name = VALUES(name), age = VALUES(age)`,
			st.ID, st.Parent, st.Name, st.Age); err != nil {
			return err
		}
	}

	if _, err := s.DB.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS = 0`); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM payment_attempts`,
		`DELETE FROM bookings`,
		`DELETE FROM seats`,
		`DELETE FROM trial_classes`,
	} {
		if _, err := s.DB.ExecContext(ctx, q); err != nil {
			_, _ = s.DB.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS = 1`)
			return err
		}
	}
	if _, err := s.DB.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS = 1`); err != nil {
		return err
	}

	classes := []struct {
		ID, Subject, Title, Teacher string
		Start                       time.Time
	}{
		{"cls_math_sat_10", "math", "Math Trial Sat 10:00", "Ms. Rivera", mathStart},
		{"cls_science_sat_11", "science", "Science Trial Sat 11:00", "Mr. Okonkwo", sciStart},
	}
	for _, c := range classes {
		if _, err := s.DB.ExecContext(ctx, `
INSERT INTO trial_classes (id, subject, title, starts_at, teacher_name, capacity, amount_cents)
VALUES (?, ?, ?, ?, ?, 4, 1000)`, c.ID, c.Subject, c.Title, c.Start, c.Teacher); err != nil {
			return err
		}
		for n := 1; n <= 4; n++ {
			if _, err := s.DB.ExecContext(ctx, `
INSERT INTO seats (id, trial_class_id, seat_number, status, booking_id, hold_expires_at)
VALUES (?, ?, ?, 'free', NULL, NULL)`, seatID(c.ID, n), c.ID, n); err != nil {
				return err
			}
		}
	}

	confirm := func(bookingID, parentID, studentID, classID string, seatNum int, when time.Time) error {
		sid := seatID(classID, seatNum)
		if _, err := s.DB.ExecContext(ctx, `
INSERT INTO bookings (
  id, parent_id, student_id, trial_class_id, seat_id, status,
  hold_expires_at, confirmed_at, amount_cents, refund_needed, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, 'confirmed', NULL, ?, 1000, 0, ?, ?)`,
			bookingID, parentID, studentID, classID, sid, when, when, when); err != nil {
			return err
		}
		_, err := s.DB.ExecContext(ctx, `
UPDATE seats SET status = 'confirmed', booking_id = ?, hold_expires_at = NULL WHERE id = ?`,
			bookingID, sid)
		return err
	}

	if err := confirm("bkg_li_math", "par_chen", "stu_li", "cls_math_sat_10", 1, now.Add(-48*time.Hour)); err != nil {
		return err
	}
	if err := confirm("bkg_arjun_math", "par_priya", "stu_arjun", "cls_math_sat_10", 2, now.Add(-36*time.Hour)); err != nil {
		return err
	}
	if err := confirm("bkg_yasmin_math", "par_omar", "stu_yasmin", "cls_math_sat_10", 3, now.Add(-24*time.Hour)); err != nil {
		return err
	}
	if err := confirm("bkg_arjun_sci", "par_priya", "stu_arjun", "cls_science_sat_11", 1, now.Add(-12*time.Hour)); err != nil {
		return err
	}

	holdExp := now.Add(s.HoldTTL)
	if _, err := s.DB.ExecContext(ctx, `
INSERT INTO bookings (
  id, parent_id, student_id, trial_class_id, seat_id, status,
  hold_expires_at, confirmed_at, amount_cents, refund_needed, created_at, updated_at
) VALUES ('bkg_sam_sci', 'par_dana', 'stu_sam', 'cls_science_sat_11', ?, 'pending_payment', ?, NULL, 1000, 0, ?, ?)`,
		seatID("cls_science_sat_11", 2), holdExp, now, now); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `
UPDATE seats SET status = 'held', booking_id = 'bkg_sam_sci', hold_expires_at = ? WHERE id = ?`,
		holdExp, seatID("cls_science_sat_11", 2)); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO payment_attempts (id, booking_id, idempotency_key, result, amount_cents, provider_reference, created_at)
VALUES ('pat_sam_fail', 'bkg_sam_sci', 'pay_sam_fail_1', 'failure', 1000, 'mock_fail', ?)`, now)
	return err
}

func seatID(classID string, n int) string {
	return fmt.Sprintf("%s_seat_%d", classID, n)
}
