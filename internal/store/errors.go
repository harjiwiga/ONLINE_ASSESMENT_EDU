package store

import "errors"

var (
	ErrUnauthorized       = errors.New("UNAUTHORIZED")
	ErrForbiddenStudent   = errors.New("FORBIDDEN_STUDENT")
	ErrNotFound           = errors.New("NOT_FOUND")
	ErrDuplicateConfirmed = errors.New("DUPLICATE_CONFIRMED")
	ErrClassFull          = errors.New("CLASS_FULL")
	ErrHoldExpired        = errors.New("HOLD_EXPIRED")
	ErrInvalidState       = errors.New("INVALID_STATE")
	ErrValidation         = errors.New("VALIDATION_ERROR")
	errReusePending       = errors.New("reuse pending after duplicate insert")
)

func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return 401
	case errors.Is(err, ErrForbiddenStudent):
		return 403
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrDuplicateConfirmed),
		errors.Is(err, ErrClassFull),
		errors.Is(err, ErrHoldExpired),
		errors.Is(err, ErrInvalidState):
		return 409
	case errors.Is(err, ErrValidation):
		return 400
	default:
		return 500
	}
}

func Code(err error) string {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return "UNAUTHORIZED"
	case errors.Is(err, ErrForbiddenStudent):
		return "FORBIDDEN_STUDENT"
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, ErrDuplicateConfirmed):
		return "DUPLICATE_CONFIRMED"
	case errors.Is(err, ErrClassFull):
		return "CLASS_FULL"
	case errors.Is(err, ErrHoldExpired):
		return "HOLD_EXPIRED"
	case errors.Is(err, ErrInvalidState):
		return "INVALID_STATE"
	case errors.Is(err, ErrValidation):
		return "VALIDATION_ERROR"
	default:
		return "INTERNAL"
	}
}
