package web

import (
	"encoding/json"
	"errors"
	"net/http"
)

type apiError struct {
	Status  int
	Message string
	Reason  string
	Err     error
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *apiError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newAPIError(status int, message string, err error) *apiError {
	return &apiError{
		Status:  status,
		Message: message,
		Err:     err,
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, status int) {
	writeAPIError(w, &apiError{Status: status, Message: message})
}

func writeAPIError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	reason := ""

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.Status != 0 {
			status = apiErr.Status
		}
		if apiErr.Message != "" {
			message = apiErr.Message
		}
		reason = apiErr.Reason
	} else if err != nil {
		message = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:  message,
		Status: status,
		Reason: reason,
	})
}
