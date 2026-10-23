package transport

import (
	"errors"
	"log"
	"net/http"

	"socrates/internal/keystore"
)

// KeyGate is what the engine serves until the database key is available (the person unlocks a passphrase or chooses where the key
// lives): the status, the key-store routes, and a 423 for everything else.
func KeyGate(store *keystore.Store, version string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, _ *http.Request) {
		code, state := http.StatusServiceUnavailable, "starting"
		if store.Status().State != keystore.StateReady {
			code, state = http.StatusLocked, "locked"
		}
		WriteJSON(w, code, map[string]string{"status": state, "version": version})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusLocked, map[string]string{"error": "Socrates is waiting for the database key."})
	})
	mux.HandleFunc("GET /api/keystore/status", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, store.Status())
	})
	mux.HandleFunc("POST /api/keystore/recheck", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, store.Recheck())
	})
	mux.HandleFunc("POST /api/keystore/unlock", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Passphrase string `json:"passphrase"`
		}
		if !ReadBody(w, r, &body) {
			return
		}
		switch err := store.Unlock(body.Passphrase); {
		case err == nil:
			WriteJSON(w, http.StatusOK, store.Status())
		case errors.Is(err, keystore.ErrWrongPassphrase):
			deny(w, http.StatusForbidden, "That passphrase is not right.")
		case errors.Is(err, keystore.ErrNotNeeded):
			deny(w, http.StatusConflict, "Socrates is not waiting for a passphrase.")
		default:
			log.Printf("[keystore] unlock failed: %v", err)
			deny(w, http.StatusInternalServerError, "The key could not be opened.")
		}
	})
	mux.HandleFunc("POST /api/keystore/setup", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode            string `json:"mode"`
			Passphrase      string `json:"passphrase"`
			ConfirmInsecure bool   `json:"confirmInsecure"`
		}
		if !ReadBody(w, r, &body) {
			return
		}
		switch err := store.Setup(body.Mode, body.Passphrase, body.ConfirmInsecure); {
		case err == nil:
			WriteJSON(w, http.StatusOK, store.Status())
		case errors.Is(err, keystore.ErrShortPassphrase), errors.Is(err, keystore.ErrNeedsConfirm), errors.Is(err, keystore.ErrNotNeeded):
			deny(w, http.StatusBadRequest, err.Error())
		default:
			log.Printf("[keystore] setup failed: %v", err)
			deny(w, http.StatusInternalServerError, "The key could not be set up.")
		}
	})
	return mux
}
