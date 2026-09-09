package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	instanceEntities "github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
)

// progress is one line of newline-delimited JSON, written while the work
// happens. A plain body would only arrive when everything was over, and the
// panel would be showing a progress bar that knows nothing.
type progress struct {
	Step     int    `json:"step,omitempty"`
	Describe string `json:"describe,omitempty"`
	Done     bool   `json:"done,omitempty"`
	Error    string `json:"error,omitempty"`
}

// reporter is what the runtime driver offers when it can narrate its work.
type reporter interface {
	CreateWithProgress(ctx context.Context, instance *instanceEntities.Instance, report func(int, string)) error
	DeleteWithProgress(ctx context.Context, name string, report func(int, string)) error
}

func (s *Server) stream(w http.ResponseWriter, run func(report func(int, string)) error) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)

	emit := func(p progress) {
		_ = encoder.Encode(p)
		if flusher != nil {
			flusher.Flush()
		}
	}

	err := run(func(step int, describe string) {
		emit(progress{Step: step, Describe: describe})
	})

	if err != nil {
		emit(progress{Error: err.Error()})
		return
	}
	emit(progress{Done: true})
}

// readProgress turns the agent's lines back into report calls on the client
// side, and returns whatever went wrong.
func readProgress(body *bufio.Reader, report func(int, string)) error {
	for {
		line, err := body.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			var p progress
			if jsonErr := json.Unmarshal([]byte(line), &p); jsonErr == nil {
				switch {
				case p.Error != "":
					return errors.New(p.Error)
				case p.Done:
					return nil
				case p.Step > 0 && report != nil:
					report(p.Step, p.Describe)
				}
			}
		}
		if err != nil {
			return err
		}
	}
}
