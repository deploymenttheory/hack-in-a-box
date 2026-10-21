package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

type errorResponse = contract.ErrorEnvelope

func errorBody(r *http.Request, code, message string) errorResponse {
	var body errorResponse
	body.Error.Code = code
	body.Error.Message = message
	body.Error.RequestId = requestID(r)
	return body
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
