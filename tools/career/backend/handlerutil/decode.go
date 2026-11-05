package handlerutil

import (
	"encoding/json"
	"net/http"
)

func DecodeJobBoardBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJobBoardBodyBytes)).Decode(into); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

const maxJobBoardBodyBytes = 16 << 10
