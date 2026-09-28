# Ottodot trial class booking

MVP slice: a parent holds a trial seat, then mock-pays. Inventory is **one MySQL row per seat**. The last seat is claimed with `SELECT ... FOR UPDATE SKIP LOCKED`. The loser is rejected **before payment**.

## Why this design

Trial classes have 4 seats. Two parents must not both get the last one, and a parent who does not get a seat must not be charged.

Shopify solved the same last-unit problem by:

1. Putting the reservation in the **same database** as inventory (not Redis + MySQL).
2. Using **one row per unit**, not a quantity column everyone locks.
3. Grabbing free units with MySQL 8 **`SKIP LOCKED`**.

This repo does the same at class scale: 4 `seats` rows, grab 1. Timed holds (8 minutes) occupy a seat until pay, expire, or cancel.

A quantity column on `trial_classes` was rejected: that is the hot-row model that buckled for Shopify.

## Stack

- Go, chi, `database/sql`
- MySQL 8 InnoDB (Docker Compose)
- Vue 3 + Vite (thin client)

## Run

MySQL 8 and Docker are required.

```bash
docker compose up -d
go run ./cmd/seed
go run ./cmd/server
```

In another terminal:

```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:5173

Demo parents: Maya, Ben, Chen, Dana (header switcher). Math Trial Sat 10:00 starts with **1 free seat** (Li, Arjun, Yasmin already confirmed). Use Maya vs Ben to demo the last-seat race.

Admin roster: http://localhost:5173/admin (`X-Admin-Role: teacher`).

API: http://localhost:8080/api/health

## Prove the last-seat invariant

```bash
docker compose up -d
go test ./...
```

`TestLastSeatRace` fires two concurrent `POST /api/bookings` against a class with one free seat. Exactly one `201` hold, one `409 CLASS_FULL`. The loser has no payment attempt.

Manual curl (after seed):

```bash
# Maya and Ben both try the last math seat
curl -s -D - -H "X-Parent-Id: par_maya" -H "Content-Type: application/json" \
  -d '{"studentId":"stu_aisha","trialClassId":"cls_math_sat_10"}' \
  http://localhost:8080/api/bookings

curl -s -D - -H "X-Parent-Id: par_ben" -H "Content-Type: application/json" \
  -d '{"studentId":"stu_noah","trialClassId":"cls_math_sat_10"}' \
  http://localhost:8080/api/bookings
```

One request gets `201 pending_payment`. The other gets `CLASS_FULL` and is not charged.

## Data model (short)

| Table | Role |
| --- | --- |
| `trial_classes` | Catalog only. **No** occupancy counter. |
| `seats` | 4 unit rows: `free` / `held` / `confirmed` |
| `bookings` | Parent workflow + hold TTL |
| `payment_attempts` | Mock charge results, idempotent keys |

Remaining seats = count of `free` seat rows after lazy expiry.

## Tradeoffs

- Docker/MySQL is heavier than SQLite. Needed for real `SKIP LOCKED`.
- Holds can block the last seat for 8 minutes. That avoids “I paid and my child is not in the class.”
- The mock gateway is not Stripe. Production should authorize, then capture only after the seat converts to `confirmed`.
- Shopify refills a ~1000-row unit pool from a ledger. We seed a fixed 4 rows per class.

## Env

See `.env.example`. Default DSN: `root:ottodot@tcp(127.0.0.1:3306)/ottodot`.
