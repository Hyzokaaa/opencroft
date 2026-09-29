package services

import (
	"strings"
	"testing"
)

// Written by an install script, then edited by hand: comments, a blank line,
// an export, quotes somebody chose.
const handWritten = `# Open Helpdesk backend
PORT=3000
export DB_HOST=localhost
APP_NAME="Open Helpdesk"

# secrets
JWT_SECRET=abc123
`

func TestAFileIsReadAsAProgramWouldReadIt(t *testing.T) {
	got := map[string]string{}
	for _, v := range ReadEnv(handWritten) {
		got[v.Key] = v.Value
	}

	want := map[string]string{
		"PORT": "3000", "DB_HOST": "localhost", "APP_NAME": "Open Helpdesk", "JWT_SECRET": "abc123",
	}
	if len(got) != len(want) {
		t.Fatalf("read %v", got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s is %q, want %q", key, got[key], value)
		}
	}
}

// Saving what was read changes nothing — not a quote, not a comment.
func TestSavingWhatWasReadChangesNothing(t *testing.T) {
	want := map[string]string{}
	for _, v := range ReadEnv(handWritten) {
		want[v.Key] = v.Value
	}
	if got := EditEnv(handWritten, want); got != handWritten {
		t.Errorf("the file changed:\n%s", got)
	}
}

// Only what was asked changes, in its place; what is gone goes; what is new
// comes at the end.
func TestOnlyWhatWasAskedChanges(t *testing.T) {
	got := EditEnv(handWritten, map[string]string{
		"PORT": "3000", "DB_HOST": "10.0.0.5", "APP_NAME": "Open Helpdesk", "SMTP_HOST": "mail",
	})

	want := `# Open Helpdesk backend
PORT=3000
export DB_HOST=10.0.0.5
APP_NAME="Open Helpdesk"

# secrets
SMTP_HOST=mail
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A value is written so that it reads back as the same value.
func TestAValueReadsBackAsItWasWritten(t *testing.T) {
	for _, value := range []string{"plain", "with space", `a"quote`, `back\slash`, "#not-a-comment", "$HOME"} {
		written := EditEnv("", map[string]string{"KEY": value})
		read := ReadEnv(written)
		if len(read) != 1 || read[0].Value != value {
			t.Errorf("%q was written as %q and read back as %v", value, strings.TrimSpace(written), read)
		}
	}
}

// When a key appears twice the last one wins, so that is the value shown —
// and saving leaves a single line for it.
func TestADuplicateKeyIsOneVariable(t *testing.T) {
	content := "MODE=a\nMODE=b\n"
	if read := ReadEnv(content); len(read) != 1 || read[0].Value != "b" {
		t.Fatalf("read %v", read)
	}
	if got := EditEnv(content, map[string]string{"MODE": "c"}); got != "MODE=c\n" {
		t.Errorf("got %q", got)
	}
}

func TestTheHashChangesWithTheContent(t *testing.T) {
	if EnvHash("A=1\n") == EnvHash("A=2\n") || EnvHash("") != EnvHash("") {
		t.Error("the hash does not tell contents apart")
	}
}
