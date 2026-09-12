package server

import (
	"context"
	"errors"
	"net/http"
)

// DNSStatus is what the panel is allowed to know: which provider is set and
// where it was read from. The values themselves never come back out.
type DNSStatus struct {
	Provider   string   `json:"provider"`
	Source     string   `json:"source,omitempty"`
	Configured bool     `json:"configured"`
	Keys       []string `json:"keys"`
}

// DNSConfig is implemented by whatever side actually holds the credentials —
// the agent when there is one, this process when there is not.
type DNSConfig interface {
	Show(ctx context.Context) (DNSStatus, error)
	Save(ctx context.Context, provider string, values map[string]string) error
}

func (d Deps) showDNS(w http.ResponseWriter, r *http.Request) {
	if d.DNS == nil {
		writeJSON(w, http.StatusOK, DNSStatus{})
		return
	}

	status, err := d.DNS.Show(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

type saveDNSRequest struct {
	Provider string            `json:"provider"`
	Values   map[string]string `json:"values"`
}

// saveDNS is write-only by design. The credentials pass through this process
// on their way to the privileged side and are never stored or read back here.
func (d Deps) saveDNS(w http.ResponseWriter, r *http.Request) {
	if d.ReadOnly {
		writeError(w, http.StatusForbidden, errors.New("this instance is read-only"))
		return
	}
	if d.DNS == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no privileged side to store credentials"))
		return
	}

	var body saveDNSRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := d.DNS.Save(r.Context(), body.Provider, body.Values); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
