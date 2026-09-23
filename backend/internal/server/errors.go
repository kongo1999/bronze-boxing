package server

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"bronzeboxing/internal/db"
)

// Stable, machine-readable error codes. Forms switch on these to show
// field-specific guidance; the human message may change, the code may not.
const (
	CodeValidation      = "VALIDATION"
	CodeInvalidID       = "INVALID_ID"
	CodeNotFound        = "NOT_FOUND"
	CodeReasonRequired  = "REASON_REQUIRED"
	CodeInsufficient    = "INSUFFICIENT_STOCK"
	CodeOverpayment     = "OVERPAYMENT"
	CodeNoCharge        = "NO_CHARGE"
	CodeAlreadyVoided   = "ALREADY_VOIDED"
	CodeVoidedLocked    = "VOIDED_LOCKED"
	CodeScheduleClash   = "SCHEDULE_CONFLICT"
	CodeCapacity        = "CAPACITY_EXCEEDED"
	CodeDuplicate       = "DUPLICATE"
	CodeTraineeNotFound = "TRAINEE_NOT_FOUND"
	CodeItemNotFound    = "ITEM_NOT_FOUND"
	CodeItemInactive    = "ITEM_INACTIVE"
	CodeDueBelowPaid    = "DUE_BELOW_PAID"
	CodeReturnExceeds   = "RETURN_EXCEEDS_SALE"
	CodeLinkedSale      = "LINKED_SALE"
	CodePlanMismatch    = "PLAN_MISMATCH"
	CodePlanFull        = "PLAN_FULL"
	CodeConflict        = "CONFLICT"
	CodeNoTransactions  = "NO_TRANSACTIONS"
	CodeUnverifiedDue   = "UNVERIFIED_DUE"
)

// APIError is an error with an HTTP status, a stable code, and optionally the
// input field it concerns and structured details (e.g. conflicting dates).
type APIError struct {
	Status  int
	Code    string
	Message string
	Field   string
	Details any
}

func (e *APIError) Error() string { return e.Message }

func apiErr(status int, code, msg string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg}
}

// badField is a 400 about one specific input field.
func badField(field, msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: CodeValidation, Message: msg, Field: field}
}

func notFound(what string) *APIError {
	return apiErr(http.StatusNotFound, CodeNotFound, what+" not found")
}

func (e *APIError) withField(f string) *APIError { e.Field = f; return e }
func (e *APIError) withDetails(d any) *APIError  { e.Details = d; return e }

// statusCode maps plain Fiber errors (and anything else) to a generic code so
// every error body has one.
func statusCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeValidation
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusConflict:
		return CodeConflict
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	default:
		return "INTERNAL"
	}
}

// errorHandler renders every error as JSON:
//
//	{ "error": "human message", "code": "STABLE_CODE", "field"?: "...", "details"?: … }
func errorHandler(c *fiber.Ctx, err error) error {
	var ae *APIError
	if errors.As(err, &ae) {
		body := fiber.Map{"error": ae.Message, "code": ae.Code}
		if ae.Field != "" {
			body["field"] = ae.Field
		}
		if ae.Details != nil {
			body["details"] = ae.Details
		}
		return c.Status(ae.Status).JSON(body)
	}
	if errors.Is(err, db.ErrNoTx) {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "The database is not running as a replica set, so money and stock changes are refused. See DEPLOY.md.",
			"code":  CodeNoTransactions,
		})
	}
	code := fiber.StatusInternalServerError
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error(), "code": statusCode(code)})
}
