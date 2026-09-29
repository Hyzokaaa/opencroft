package services

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// An environment file is the source of truth for a service's environment: the
// one its unit reads at start, or its build reads when it runs. croft is an
// editor of that file, never the keeper of a second copy — so a change made
// over ssh is what the panel shows next, and nothing croft does later can put
// an older value back.
//
// Editing it changes only what was asked. Comments, blank lines, the order,
// an `export` in front, the quotes somebody chose: all of it stays exactly as
// it was, because the file belongs to whoever wrote it.

var envLine = regexp.MustCompile(`^(\s*)(export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

// EnvVar is one variable, its value as a program reading the file sees it.
type EnvVar struct {
	Key   string
	Value string
}

// ReadEnv lists the variables in a file. A key that appears twice is listed
// once, with its last value — which is the one that wins.
func ReadEnv(content string) []EnvVar {
	values := map[string]string{}
	order := []string{}

	for _, line := range strings.Split(content, "\n") {
		m := envLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		if _, seen := values[m[3]]; !seen {
			order = append(order, m[3])
		}
		values[m[3]] = unquote(m[4])
	}

	found := make([]EnvVar, len(order))
	for i, key := range order {
		found[i] = EnvVar{Key: key, Value: values[key]}
	}
	return found
}

// EditEnv makes a file say want. A variable whose value did not change keeps
// its line exactly as written; one that did keeps its place; one no longer
// wanted goes; a new one is added at the end. Everything that is not a
// variable is left alone.
func EditEnv(content string, want map[string]string) string {
	current := map[string]string{}
	for _, v := range ReadEnv(content) {
		current[v.Key] = v.Value
	}

	out := []string{}
	written := map[string]bool{}

	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		m := envLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			if line != "" || len(out) > 0 {
				out = append(out, line)
			}
			continue
		}

		key := m[3]
		value, wanted := want[key]
		if !wanted || written[key] {
			continue
		}
		written[key] = true

		if value == current[key] {
			out = append(out, line)
		} else {
			out = append(out, m[1]+m[2]+key+"="+quote(value))
		}
	}

	added := []string{}
	for key := range want {
		if !written[key] {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	for _, key := range added {
		out = append(out, key+"="+quote(want[key]))
	}

	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// EnvHash identifies exactly what a file held when it was read, so that a save
// can tell whether somebody changed it in the meantime.
func EnvHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func unquote(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		inner := value[1 : len(value)-1]
		if value[0] == '"' {
			inner = strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(inner)
		}
		return inner
	}
	return value
}

// quote writes a value the way both systemd and a dotenv reader take back
// unchanged: bare when nothing in it could be misread, in double quotes when
// something could.
func quote(value string) string {
	if !strings.ContainsAny(value, " \t#\"'\\$`") {
		return value
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
