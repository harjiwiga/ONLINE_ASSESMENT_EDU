package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	httpx "online_assesment_edu/internal/http"
	"online_assesment_edu/internal/payment"
	"online_assesment_edu/internal/store"
)

func testDSN() string {
	if v := os.Getenv("MYSQL_TEST_DSN"); v != "" {
		return v
	}
	return "root:ottodot@tcp(127.0.0.1:3306)/ottodot_test?parseTime=true&loc=UTC&timeout=10s"
}

func setup(t *testing.T) (*store.Store, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(testDSN(), 8*time.Minute)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mig := findMigrations(root)
	if err := st.ResetSchema(ctx, mig); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := st.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv := httptest.NewServer(httpx.New(st, payment.Scripted{}))
	t.Cleanup(srv.Close)
	return st, srv
}

func findMigrations(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = start
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "migrations")
		if _, err := os.Stat(filepath.Join(p, "001_init.sql")); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "migrations"
}

func doJSON(t *testing.T, method, url, parent string, body any, headers map[string]string) (int, map[string]any, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if parent != "" {
		req.Header.Set("X-Parent-Id", parent)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var m map[string]any
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &m)
	}
	return res.StatusCode, m, raw
}

func TestHealth(t *testing.T) {
	_, srv := setup(t)
	code, body, _ := doJSON(t, http.MethodGet, srv.URL+"/api/health", "", nil, nil)
	if code != 200 || body["status"] != "ok" {
		t.Fatalf("health %d %#v", code, body)
	}
}

func TestHappyPath(t *testing.T) {
	st, srv := setup(t)
	code, created, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", map[string]string{
		"studentId":    "stu_aisha",
		"trialClassId": "cls_science_sat_11",
	}, nil)
	if code != 201 {
		t.Fatalf("create %d %#v", code, created)
	}
	id := created["id"].(string)
	if created["status"] != "pending_payment" {
		t.Fatalf("status %v", created["status"])
	}
	code, paid, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings/"+id+"/pay", "par_maya", map[string]string{
		"idempotencyKey": "pay_aisha_happy",
		"outcome":        "success",
	}, nil)
	if code != 200 || paid["status"] != "confirmed" {
		t.Fatalf("pay %d %#v", code, paid)
	}
	code, roster, _ := doJSON(t, http.MethodGet, srv.URL+"/api/admin/trial-classes/cls_science_sat_11/roster", "", nil, map[string]string{
		"X-Admin-Role": "teacher",
	})
	if code != 200 {
		t.Fatalf("roster %d %#v", code, roster)
	}
	students, _ := roster["students"].([]any)
	found := false
	for _, s := range students {
		m := s.(map[string]any)
		if m["studentId"] == "stu_aisha" {
			found = true
		}
	}
	if !found {
		t.Fatalf("aisha not on roster %#v", roster)
	}
	c, h, f, err := st.Occupancy(context.Background(), "cls_science_sat_11")
	if err != nil {
		t.Fatal(err)
	}
	if c < 2 || h != 1 || c+h+f != 4 {
		t.Fatalf("occupancy confirmed=%d held=%d free=%d", c, h, f)
	}
}

func TestDuplicateConfirmed(t *testing.T) {
	_, srv := setup(t)
	code, body, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_chen", map[string]string{
		"studentId":    "stu_li",
		"trialClassId": "cls_math_sat_10",
	}, nil)
	if code != 409 || body["code"] != "DUPLICATE_CONFIRMED" {
		t.Fatalf("expected duplicate, got %d %#v", code, body)
	}
}

func TestPaymentFailureKeepsHold(t *testing.T) {
	st, srv := setup(t)
	code, created, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", map[string]string{
		"studentId":    "stu_aisha",
		"trialClassId": "cls_science_sat_11",
	}, nil)
	if code != 201 {
		t.Fatalf("create %d %#v", code, created)
	}
	id := created["id"].(string)
	code, paid, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings/"+id+"/pay", "par_maya", map[string]string{
		"idempotencyKey": "pay_aisha_fail",
		"outcome":        "failure",
	}, nil)
	if code != 200 || paid["status"] != "pending_payment" {
		t.Fatalf("fail pay %d %#v", code, paid)
	}
	if paid["lastPaymentResult"] != "failure" {
		t.Fatalf("last result %#v", paid)
	}
	_, held, _, err := st.Occupancy(context.Background(), "cls_science_sat_11")
	if err != nil {
		t.Fatal(err)
	}
	if held < 2 {
		t.Fatalf("hold released after failure, held=%d", held)
	}
}

func TestForbiddenStudent(t *testing.T) {
	_, srv := setup(t)
	code, body, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", map[string]string{
		"studentId":    "stu_noah",
		"trialClassId": "cls_science_sat_11",
	}, nil)
	if code != 403 || body["code"] != "FORBIDDEN_STUDENT" {
		t.Fatalf("got %d %#v", code, body)
	}
}

func TestReusePending(t *testing.T) {
	_, srv := setup(t)
	body := map[string]string{"studentId": "stu_aisha", "trialClassId": "cls_science_sat_11"}
	code1, a, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", body, nil)
	code2, b, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", body, nil)
	if code1 != 201 || code2 != 200 {
		t.Fatalf("codes %d %d", code1, code2)
	}
	if a["id"] != b["id"] {
		t.Fatalf("different bookings %v %v", a["id"], b["id"])
	}
}

func TestIdempotentPay(t *testing.T) {
	_, srv := setup(t)
	_, created, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", map[string]string{
		"studentId": "stu_aisha", "trialClassId": "cls_science_sat_11",
	}, nil)
	id := created["id"].(string)
	payload := map[string]string{"idempotencyKey": "same-key", "outcome": "success"}
	_, p1, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings/"+id+"/pay", "par_maya", payload, nil)
	_, p2, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings/"+id+"/pay", "par_maya", payload, nil)
	if p1["status"] != "confirmed" || p2["status"] != "confirmed" {
		t.Fatalf("%v %v", p1, p2)
	}
}

func TestLastSeatRace(t *testing.T) {
	st, srv := setup(t)
	var codes []int
	var bodies []map[string]any
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	run := func(parent, student string) {
		defer wg.Done()
		code, body, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", parent, map[string]string{
			"studentId": student, "trialClassId": "cls_math_sat_10",
		}, nil)
		mu.Lock()
		codes = append(codes, code)
		bodies = append(bodies, body)
		mu.Unlock()
	}
	go run("par_maya", "stu_aisha")
	go run("par_ben", "stu_noah")
	wg.Wait()
	var created, full int
	var winner map[string]any
	for i, c := range codes {
		switch c {
		case 201:
			created++
			winner = bodies[i]
		case 409:
			full++
			if bodies[i]["code"] != "CLASS_FULL" {
				t.Fatalf("loser body %#v", bodies[i])
			}
		default:
			t.Fatalf("unexpected code %d %#v", c, bodies[i])
		}
	}
	if created != 1 || full != 1 {
		t.Fatalf("want 1 created 1 full, got created=%d full=%d codes=%v", created, full, codes)
	}
	c, h, f, err := st.Occupancy(context.Background(), "cls_math_sat_10")
	if err != nil {
		t.Fatal(err)
	}
	if c != 3 || h != 1 || f != 0 {
		t.Fatalf("occupancy confirmed=%d held=%d free=%d", c, h, f)
	}
	n, err := st.CountPaymentAttempts(context.Background(), "no-such")
	if err != nil {
		t.Fatal(err)
	}
	_ = n
	if winner["status"] != "pending_payment" {
		t.Fatalf("winner %#v", winner)
	}
	code, paid, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings/"+winner["id"].(string)+"/pay", winner["parentId"].(string), map[string]string{
		"idempotencyKey": "pay_winner",
		"outcome":        "success",
	}, nil)
	if code != 200 || paid["status"] != "confirmed" {
		t.Fatalf("winner pay %d %#v", code, paid)
	}
	c, h, f, _ = st.Occupancy(context.Background(), "cls_math_sat_10")
	if c != 4 || h != 0 || f != 0 {
		t.Fatalf("after pay confirmed=%d held=%d free=%d", c, h, f)
	}
}

func TestOverbookSingleThread(t *testing.T) {
	_, srv := setup(t)
	code, _, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", map[string]string{
		"studentId": "stu_aisha", "trialClassId": "cls_math_sat_10",
	}, nil)
	if code != 201 {
		t.Fatalf("first last seat %d", code)
	}
	code, body, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_ben", map[string]string{
		"studentId": "stu_noah", "trialClassId": "cls_math_sat_10",
	}, nil)
	if code != 409 || body["code"] != "CLASS_FULL" {
		t.Fatalf("second %d %#v", code, body)
	}
}

func TestHoldExpiry(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(testDSN(), 80*time.Millisecond)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	defer st.Close()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ResetSchema(ctx, findMigrations(wd)); err != nil {
		t.Fatal(err)
	}
	if err := st.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := st.CreateBooking(ctx, "par_maya", "stu_aisha", "cls_science_sat_11")
	if err != nil {
		t.Fatal(err)
	}
	if res.Booking.Status != store.StatusPending {
		t.Fatalf("status %s", res.Booking.Status)
	}
	time.Sleep(150 * time.Millisecond)
	if err := st.SweepExpired(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := st.GetBooking(ctx, res.Booking.ID, "par_maya")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != store.StatusExpired {
		t.Fatalf("want expired got %s", b.Status)
	}
	_, _, free, err := st.Occupancy(ctx, "cls_science_sat_11")
	if err != nil {
		t.Fatal(err)
	}
	if free < 2 {
		t.Fatalf("seat not released, free=%d", free)
	}
	res2, err := st.CreateBooking(ctx, "par_ben", "stu_noah", "cls_science_sat_11")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Created {
		t.Fatalf("expected new hold after expiry")
	}
}

func TestRosterFiltersNonConfirmed(t *testing.T) {
	_, srv := setup(t)
	code, roster, _ := doJSON(t, http.MethodGet, srv.URL+"/api/admin/trial-classes/cls_science_sat_11/roster", "", nil, map[string]string{
		"X-Admin-Role": "teacher",
	})
	if code != 200 {
		t.Fatalf("%d %#v", code, roster)
	}
	students, _ := roster["students"].([]any)
	for _, s := range students {
		m := s.(map[string]any)
		if m["studentId"] == "stu_sam" {
			t.Fatalf("sam (held/failed pay) on roster")
		}
	}
}

func TestUnknownClassRoster(t *testing.T) {
	_, srv := setup(t)
	code, body, _ := doJSON(t, http.MethodGet, srv.URL+"/api/admin/trial-classes/nope/roster", "", nil, map[string]string{
		"X-Admin-Role": "teacher",
	})
	if code != 404 || body["code"] != "NOT_FOUND" {
		t.Fatalf("%d %#v", code, body)
	}
}

func TestCancelReleasesSeat(t *testing.T) {
	st, srv := setup(t)
	_, created, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings", "par_maya", map[string]string{
		"studentId": "stu_aisha", "trialClassId": "cls_science_sat_11",
	}, nil)
	id := created["id"].(string)
	code, cancelled, _ := doJSON(t, http.MethodPost, srv.URL+"/api/bookings/"+id+"/cancel", "par_maya", map[string]any{}, nil)
	if code != 200 || cancelled["status"] != "cancelled" {
		t.Fatalf("cancel %d %#v", code, cancelled)
	}
	_, held, _, _ := st.Occupancy(context.Background(), "cls_science_sat_11")
	if held != 1 {
		t.Fatalf("held=%d want 1 (sam only)", held)
	}
}

func TestRaceLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("iter-%d", i), TestLastSeatRace)
	}
}
