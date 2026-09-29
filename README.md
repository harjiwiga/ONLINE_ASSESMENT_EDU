# Ottodot trial class booking

Parents book a 4-seat trial class. The hard rule: two parents must not both get the last seat, and a parent who does not get a seat must not be charged.

## What I built

A small working slice: **hold a seat, then mock-pay, then confirm**.

- Go API (chi) + MySQL 8 InnoDB
- Vue 3 parent booking UI and a teacher roster
- Timed 8-minute hold on submit
- Mock pay success / fail
- Confirmed roster (holds and failed payments stay off it)
- Concurrent last-seat test against real MySQL

Inventory is **one row per seat** (4 rows per class). The last free row is claimed with `SELECT … FOR UPDATE SKIP LOCKED`. Payment only converts that same row from `held` to `confirmed`. It does not compete for a seat.

Details: [PRD.md](./PRD.md), [TSD.md](./TSD.md).

## Time spent

About **3-5 hours**:


| Block | What                                                    |
| ----- | ------------------------------------------------------- |
| ~1h   | PRD / TSD: last-seat rule, hold-then-pay, what to cut   |
| ~30m  | Review TSD and PRD                                      |
| ~30m  | Schema, hold claim, pay/cancel/expiry, seed, race tests |
| ~30m  | Manual Testing                                          |
| ~1–2h | Create demo video                                       |


## How to run

Needs **Docker** and **MySQL 8**.

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

Open [http://localhost:5173](http://localhost:5173)

- Header parent switcher: Maya, Ben, Chen, Dana
- **Math Trial Sat 10:00** starts with **1 free seat** (Li, Arjun, Yasmin already confirmed). Maya vs Ben is the last-seat demo.
- Admin roster: [http://localhost:5173/admin](http://localhost:5173/admin) (`X-Admin-Role: teacher`)
- API health: [http://localhost:8080/api/health](http://localhost:8080/api/health)

Env: `[.env.example](./.env.example)`. Default DSN `root:ottodot@tcp(127.0.0.1:3306)/ottodot`.

Reset demo data anytime (server can stay up):

```bash
go run ./cmd/seed
```

### Prove the last-seat rule

```bash
docker compose up -d
go test ./...
```

`TestLastSeatRace` fires two concurrent `POST /api/bookings`. Exactly one `201` hold, one `409 CLASS_FULL`. The loser has no payment attempt.

Manual (after seed). One `201 pending_payment`, one `CLASS_FULL`, no charge for the loser:

```bash
curl -s -o /tmp/maya.json -w "maya %{http_code}\n" \
  -H "X-Parent-Id: par_maya" -H "Content-Type: application/json" \
  -d '{"studentId":"stu_aisha","trialClassId":"cls_math_sat_10"}' \
  http://localhost:8080/api/bookings &
curl -s -o /tmp/ben.json -w "ben %{http_code}\n" \
  -H "X-Parent-Id: par_ben" -H "Content-Type: application/json" \
  -d '{"studentId":"stu_noah","trialClassId":"cls_math_sat_10"}' \
  http://localhost:8080/api/bookings &
wait
cat /tmp/maya.json; echo; cat /tmp/ben.json; echo
```

Roster still shows 3 until the winner **pays**. A hold occupies the 4th seat; it is not a confirmed student.

Failed payment: on checkout click **Try declined card**, or `POST /api/bookings/:id/pay` with `"outcome":"failure"`. Booking stays `pending_payment`; child stays off the roster.

## Assumptions I made

- Capacity is always **4**. Each class is seeded with **4 `seats` rows**, status `free`. Booking does not insert a 5th row.
- The last-seat winner is decided **at hold create**, not after money moves.
- UI “seats left” can be stale. MySQL claim is the source of truth.
- Demo identity is a header (`X-Parent-Id` / `X-Admin-Role`), not signup.
- Mock pay is enough if it **refuses to charge without a valid hold**.
- Failed card **keeps the hold** until expiry so the parent can retry.
- One child per booking. Same child cannot have two active bookings on the same class.
- Lazy expiry on hold/list/pay is required. A background sweep is extra, not the only release path.
- Reviewer has Docker so `SKIP LOCKED` is real MySQL 8, not a SQLite fake.

## Key architecture and backend decisions

Go owns correctness. Vue is a thin client. No seat logic in the browser.


| Decision                                                        | Why                                                                                                                           |
| --------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| One InnoDB **row per seat**, not a quantity column on the class | A `held_count` cell is a hot row. Everyone locks the same counter.                                                            |
| `SELECT … FOR UPDATE SKIP LOCKED LIMIT 1` at create             | Lock one `free` row. If it is already locked, skip it. On the last seat the loser gets 0 rows → `CLASS_FULL`, never a charge. |
| Hold and booking insert in **one transaction**                  | Seat and booking cannot drift.                                                                                                |
| Mock gateway **outside** that transaction                       | Do not hold DB locks during “payment”.                                                                                        |
| Pay only **converts** `held` → `confirmed`                      | No second inventory grab.                                                                                                     |
| Occupied = `confirmed` + `held`                                 | A hold blocks the last seat. Roster is confirmed only.                                                                        |
| No Redis for reservations                                       | Two stores, no shared transaction → oversell / undersell.                                                                     |
| Postgres `SKIP LOCKED` would work the same                      | MySQL 8 was chosen so the lock feature matches the unit-row pattern.                                                          |


Flow in one line: **expire stale holds → grab one free seat or `CLASS_FULL` → pending_payment → pay success converts that seat**.

## What I deliberately cut

Out of scope for this slice (see PRD non-goals):

- Real Stripe / PSP (mock only; production should authorize then capture after confirm)
- Parent signup, email/password, OTP
- Waitlist, auto-promote, rebooking
- Notifications
- Multi-child checkout, promos, term enrollment
- Executing refunds (only record `refund_needed` if a late charge slips through)
- Polished marketing UI, i18n
- Serving Vue from the Go binary

Also rejected on purpose: let both parents pay, then tell the loser “we charged you but the class is full.”

## What I would monitor after release


| Signal                                         | Why it matters                                           |
| ---------------------------------------------- | -------------------------------------------------------- |
| `CLASS_FULL` on create vs pay                  | Loser must fail at **hold**, not after charge            |
| Confirmed + held vs 4 seat rows                | Occupancy never above 4                                  |
| Confirmed roster vs `seats.status = confirmed` | Drift = a bug                                            |
| Hold created but never paid (expiry rate)      | Abandoned last seats                                     |
| Payment `failure` then retry `success`         | Fail-pay path is working                                 |
| `refund_needed > 0`                            | Hold expired between charge and convert — should be rare |
| Duplicate `active_pair` unique conflicts       | Same child double-book                                   |
| Create latency under concurrent load           | `SKIP LOCKED` staying healthy, not a hot-row wait        |


Alert if a class ever has more than 4 `held`+`confirmed` seats, or a successful payment with no matching confirmed seat.

## What I would do next with more time

1. Real payments: authorize on hold, capture only after the seat is `confirmed`.
2. Real auth instead of demo headers.
3. Hold TTL from product (5 vs 8 vs 10 minutes), plus a waitlist if a hold expires.
4. Metrics/tracing on the claim query and pay convert.
5. Email when hold is about to expire or class is full.
6. One-binary demo: Go serves `frontend/dist`.
7. If the reviewer prefers Postgres, same seat-row + `SKIP LOCKED` SQL.

## Env

See `.env.example`. `HOLD_TTL` defaults to `8m`.