CREATE TABLE IF NOT EXISTS parents (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  email VARCHAR(255) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS students (
  id VARCHAR(64) PRIMARY KEY,
  parent_id VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  age INT NULL,
  CONSTRAINT fk_students_parent FOREIGN KEY (parent_id) REFERENCES parents(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS trial_classes (
  id VARCHAR(64) PRIMARY KEY,
  subject VARCHAR(32) NOT NULL,
  title VARCHAR(255) NOT NULL,
  starts_at DATETIME(3) NOT NULL,
  teacher_name VARCHAR(255) NOT NULL,
  capacity INT NOT NULL,
  amount_cents INT NOT NULL,
  CONSTRAINT chk_class_capacity CHECK (capacity = 4),
  CONSTRAINT chk_class_amount CHECK (amount_cents > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS seats (
  id VARCHAR(64) PRIMARY KEY,
  trial_class_id VARCHAR(64) NOT NULL,
  seat_number INT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'free',
  booking_id VARCHAR(64) NULL,
  hold_expires_at DATETIME(3) NULL,
  UNIQUE KEY uq_class_seat (trial_class_id, seat_number),
  UNIQUE KEY uq_seat_booking (booking_id),
  KEY seats_class_status (trial_class_id, status),
  CONSTRAINT chk_seat_number CHECK (seat_number BETWEEN 1 AND 4),
  CONSTRAINT chk_seat_status CHECK (status IN ('free', 'held', 'confirmed')),
  CONSTRAINT chk_seat_held CHECK (status <> 'held' OR (booking_id IS NOT NULL AND hold_expires_at IS NOT NULL)),
  CONSTRAINT chk_seat_confirmed CHECK (status <> 'confirmed' OR booking_id IS NOT NULL),
  CONSTRAINT fk_seats_class FOREIGN KEY (trial_class_id) REFERENCES trial_classes(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bookings (
  id VARCHAR(64) PRIMARY KEY,
  parent_id VARCHAR(64) NOT NULL,
  student_id VARCHAR(64) NOT NULL,
  trial_class_id VARCHAR(64) NOT NULL,
  seat_id VARCHAR(64) NULL,
  status VARCHAR(32) NOT NULL,
  hold_expires_at DATETIME(3) NULL,
  confirmed_at DATETIME(3) NULL,
  amount_cents INT NOT NULL,
  refund_needed TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  active_pair VARCHAR(128) GENERATED ALWAYS AS (
    CASE
      WHEN status IN ('confirmed', 'pending_payment') THEN CONCAT(student_id, ':', trial_class_id)
      ELSE NULL
    END
  ) STORED,
  UNIQUE KEY bookings_one_active (active_pair),
  KEY bookings_class_status (trial_class_id, status),
  CONSTRAINT chk_booking_status CHECK (status IN (
    'pending_payment', 'confirmed', 'expired', 'cancelled', 'seat_unavailable'
  )),
  CONSTRAINT chk_booking_hold CHECK (status <> 'pending_payment' OR hold_expires_at IS NOT NULL),
  CONSTRAINT chk_booking_confirmed CHECK (status <> 'confirmed' OR confirmed_at IS NOT NULL),
  CONSTRAINT chk_booking_amount CHECK (amount_cents > 0),
  CONSTRAINT fk_bookings_parent FOREIGN KEY (parent_id) REFERENCES parents(id),
  CONSTRAINT fk_bookings_student FOREIGN KEY (student_id) REFERENCES students(id),
  CONSTRAINT fk_bookings_class FOREIGN KEY (trial_class_id) REFERENCES trial_classes(id),
  CONSTRAINT fk_bookings_seat FOREIGN KEY (seat_id) REFERENCES seats(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE seats
  ADD CONSTRAINT fk_seats_booking FOREIGN KEY (booking_id) REFERENCES bookings(id);


CREATE TABLE IF NOT EXISTS payment_attempts (
  id VARCHAR(64) PRIMARY KEY,
  booking_id VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  result VARCHAR(16) NOT NULL,
  amount_cents INT NOT NULL,
  provider_reference VARCHAR(128) NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uq_idempotency (idempotency_key),
  CONSTRAINT chk_attempt_result CHECK (result IN ('success', 'failure', 'pending')),
  CONSTRAINT fk_attempts_booking FOREIGN KEY (booking_id) REFERENCES bookings(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
