package agent

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TipDTO is the commit at the end of the branch a service deploys from, as
// the repository says now.
type TipDTO struct {
	Commit string `json:"commit"`
}

// tipPatience bounds the question: a repository that does not answer is
// asked again next time, not waited on.
const tipPatience = 30 * time.Second

// tip asks the repository for the end of the branch, without fetching
// anything: `git ls-remote` reads the list of branches and nothing else. It
// is asked from inside the container, which has git and trusts what its
// deployments needed it to trust; the host may have neither.
func (s *Server) tip(w http.ResponseWriter, r *http.Request) {
	name, service, err := s.stored(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	branch := service.Source.Branch
	if branch == "" {
		branch = "main"
	}

	ctx, cancel := context.WithTimeout(r.Context(), tipPatience)
	defer cancel()
	// Arguments, not a shell line: the URL and the branch were checked
	// when they were saved, and here they could not be anything else anyway.
	out, err := s.host.Run(ctx, s.bin, "exec", name, "--",
		"git", "ls-remote", service.Source.Repo, "refs/heads/"+branch)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	commit := strings.Fields(out.Stdout)
	if len(commit) == 0 {
		writeError(w, http.StatusNotFound, errors.New(service.Source.Repo+" has no branch "+branch))
		return
	}
	writeJSON(w, http.StatusOK, TipDTO{Commit: commit[0]})
}

func (a *ServiceClient) Tip(ctx context.Context, container, service string) (string, error) {
	var response TipDTO
	err := a.c.call(ctx, http.MethodGet,
		servicePath(container, "/"+url.PathEscape(service)+"/tip"), nil, &response)
	return response.Commit, err
}
