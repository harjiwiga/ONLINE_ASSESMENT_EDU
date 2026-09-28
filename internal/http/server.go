package httpx

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"online_assesment_edu/internal/payment"
	"online_assesment_edu/internal/store"
)

type Server struct {
	Store   *store.Store
	Gateway payment.Gateway
}

func New(st *store.Store, gw payment.Gateway) http.Handler {
	if gw == nil {
		gw = payment.Scripted{}
	}
	s := &Server{Store: st, Gateway: gw}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors)

	r.Get("/api/health", s.health)
	r.Get("/api/me/students", s.listStudents)
	r.Get("/api/trial-classes", s.listClasses)
	r.Post("/api/bookings", s.createBooking)
	r.Get("/api/bookings/{id}", s.getBooking)
	r.Post("/api/bookings/{id}/pay", s.payBooking)
	r.Post("/api/bookings/{id}/cancel", s.cancelBooking)
	r.Get("/api/admin/trial-classes/{id}/roster", s.roster)

	dist := filepath.Join("frontend", "dist")
	if info, err := os.Stat(dist); err == nil && info.IsDir() {
		fileServer(r, "/", http.Dir(dist))
	}
	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Parent-Id, X-Admin-Role")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fileServer(r chi.Router, path string, root http.FileSystem) {
	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"
	r.Get(path, func(w http.ResponseWriter, req *http.Request) {
		fs := http.StripPrefix("/", http.FileServer(root))
		fs.ServeHTTP(w, req)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Ping(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listStudents(w http.ResponseWriter, r *http.Request) {
	parentID, err := requireParent(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.Store.ListStudents(r.Context(), parentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, st := range list {
		item := map[string]any{"id": st.ID, "parentId": st.ParentID, "name": st.Name}
		if st.Age.Valid {
			item["age"] = st.Age.Int64
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listClasses(w http.ResponseWriter, r *http.Request) {
	if _, err := requireParent(r); err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.Store.ListClasses(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{
			"id":             c.ID,
			"subject":        c.Subject,
			"title":          c.Title,
			"startsAt":       c.StartsAt.UTC().Format(time.RFC3339Nano),
			"teacherName":    c.TeacherName,
			"capacity":       c.Capacity,
			"confirmedCount": c.ConfirmedCount,
			"heldCount":      c.HeldCount,
			"remainingSeats": c.RemainingSeats,
			"amountCents":    c.AmountCents,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type createBody struct {
	StudentID    string `json:"studentId"`
	TrialClassID string `json:"trialClassId"`
}

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) {
	parentID, err := requireParent(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body createBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, store.ErrValidation)
		return
	}
	res, err := s.Store.CreateBooking(r.Context(), parentID, body.StudentID, body.TrialClassID)
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, bookingJSON(res.Booking))
}

func (s *Server) getBooking(w http.ResponseWriter, r *http.Request) {
	parentID, err := requireParent(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	b, err := s.Store.GetBooking(r.Context(), chi.URLParam(r, "id"), parentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookingJSON(b))
}

type payBody struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Outcome        string `json:"outcome"`
}

func (s *Server) payBooking(w http.ResponseWriter, r *http.Request) {
	parentID, err := requireParent(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body payBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, store.ErrValidation)
		return
	}
	b, err := s.Store.PayForBooking(r.Context(), store.PayInput{
		BookingID:      chi.URLParam(r, "id"),
		ParentID:       parentID,
		IdempotencyKey: body.IdempotencyKey,
		Outcome:        body.Outcome,
		Gateway:        s.Gateway,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookingJSON(b))
}

func (s *Server) cancelBooking(w http.ResponseWriter, r *http.Request) {
	parentID, err := requireParent(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	b, err := s.Store.CancelBooking(r.Context(), chi.URLParam(r, "id"), parentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookingJSON(b))
}

func (s *Server) roster(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Admin-Role") != "teacher" {
		writeErr(w, store.ErrUnauthorized)
		return
	}
	ros, err := s.Store.GetRoster(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	students := make([]map[string]any, 0, len(ros.Students))
	for _, e := range ros.Students {
		students = append(students, map[string]any{
			"bookingId":   e.BookingID,
			"studentId":   e.StudentID,
			"studentName": e.StudentName,
			"parentName":  e.ParentName,
			"confirmedAt": e.ConfirmedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trialClassId":   ros.TrialClassID,
		"title":          ros.Title,
		"capacity":       ros.Capacity,
		"confirmedCount": ros.ConfirmedCount,
		"heldCount":      ros.HeldCount,
		"remainingSeats": ros.RemainingSeats,
		"students":       students,
	})
}

func bookingJSON(b *store.Booking) map[string]any {
	m := map[string]any{
		"id":            b.ID,
		"parentId":      b.ParentID,
		"studentId":     b.StudentID,
		"studentName":   b.StudentName,
		"parentName":    b.ParentName,
		"trialClassId":  b.TrialClassID,
		"classTitle":    b.ClassTitle,
		"classSubject":  b.ClassSubject,
		"teacherName":   b.TeacherName,
		"classStartsAt": b.ClassStartsAt.UTC().Format(time.RFC3339Nano),
		"status":        b.Status,
		"amountCents":   b.AmountCents,
		"refundNeeded":  b.RefundNeeded,
		"createdAt":     b.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":     b.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if b.SeatID.Valid {
		m["seatId"] = b.SeatID.String
	}
	if b.HoldExpiresAt.Valid {
		m["holdExpiresAt"] = b.HoldExpiresAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if b.ConfirmedAt.Valid {
		m["confirmedAt"] = b.ConfirmedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if b.LastPaymentResult.Valid {
		m["lastPaymentResult"] = b.LastPaymentResult.String
	}
	return m
}

func requireParent(r *http.Request) (string, error) {
	id := strings.TrimSpace(r.Header.Get("X-Parent-Id"))
	if id == "" {
		return "", store.ErrUnauthorized
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	msg := err.Error()
	code := store.Code(err)
	status := store.HTTPStatus(err)
	if status == 500 {
		log.Printf("internal error: %v", err)
		msg = "internal error"
	}
	if errors.Is(err, store.ErrClassFull) {
		msg = "That seat was just taken. You were not charged."
	}
	if errors.Is(err, store.ErrHoldExpired) {
		msg = "The seat hold expired. You were not confirmed."
	}
	if errors.Is(err, store.ErrDuplicateConfirmed) {
		msg = "This child is already confirmed on the class."
	}
	writeJSON(w, status, map[string]string{"code": code, "message": msg})
}
