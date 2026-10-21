package httpapi

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func errorBody(r *http.Request, code, message string) errorResponse {
	return errorResponse{Error: errorDetail{Code: code, Message: message, RequestID: requestID(r)}}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorBody(r, code, message))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		// Encoding errors are programming errors; recovery emits a safe envelope.
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A disconnected client cannot receive another error response.
	_, _ = w.Write(append(data, '\n'))
}
