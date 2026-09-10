package agent

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Hyzokaaa/opencroft/internal/certificate/infrastructure/acme"
)

// The credentials live on this side of the socket and stay here. The panel can
// write them and can ask whether any exist; it can never read them back.
func (s *Server) showDNS(w http.ResponseWriter, r *http.Request) {
	response := DNSCredentialsDTO{Keys: acme.Keys("ovh")}

	credentials, err := acme.LoadCredentials()
	if err == nil {
		response.Provider = credentials.Provider
		response.Source = credentials.Source
		response.Configured = true
		response.Keys = acme.Keys(credentials.Provider)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) saveDNS(w http.ResponseWriter, r *http.Request) {
	var dto DNSCredentialsDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if acme.Keys(dto.Provider) == nil {
		writeError(w, http.StatusBadRequest,
			errors.New("croft knows the DNS providers ovh and cloudflare"))
		return
	}
	if len(dto.Values) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("no credentials were sent"))
		return
	}

	if err := acme.Save(dto.Provider, dto.Values); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
