package term

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

func TestColorEnabled(t *testing.T) {
	cases := []struct {
		name     string
		terminal bool
		env      map[string]string
		want     bool
	}{
		{"terminal with xterm", true, map[string]string{"TERM": "xterm-256color"}, true},
		{"terminal without TERM", true, nil, true},
		{"not a terminal", false, map[string]string{"TERM": "xterm"}, false},
		{"NO_COLOR set", true, map[string]string{"NO_COLOR": "1"}, false},
		{"NO_COLOR empty counts as unset", true, map[string]string{"NO_COLOR": ""}, true},
		{"TERM dumb", true, map[string]string{"TERM": "dumb"}, false},
		{"piped with NO_COLOR", false, map[string]string{"NO_COLOR": "1"}, false},
	}
	for _, c := range cases {
		if got := ColorEnabled(c.terminal, env(c.env)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClean(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"plain", "Dev Team", 0, "Dev Team"},
		{"esc color", "Dev\u001b[31m Team", 0, "Dev Team"},
		{"csi with params", "a\u001b[1;38;5;196mb\u001b[0mc", 0, "abc"},
		{"osc bel", "x\u001b]0;evil title\u0007y", 0, "xy"},
		{"osc st", "x\u001b]8;;http://e\u001b\\y", 0, "xy"},
		{"unterminated osc", "keep\u001b]0;never ends", 0, "keep"},
		{"lone esc at end", "end\u001b", 0, "end"},
		{"other escape", "a\u001bcb", 0, "ab"},
		{"newline and tab", "one\ntwo\tthree\r\nfour", 0, "one two three four"},
		{"collapse spaces", "  a    b  ", 0, "a b"},
		{"c0 controls", "a\u0000b\u0007c\u007fd", 0, "abcd"},
		{"c1 controls", "a\u0085b\u0080c\u0099d", 0, "a bcd"},
		{"8-bit osc", "a\u009d0;title\u009cb", 0, "ab"},
		{"dcs string", "a\u001bPq#0;payload\u001b\\b", 0, "ab"},
		{"apc string", "a\u001b_hidden\u0007b", 0, "ab"},
		{"bidi override", "invoice\u202Egnp.exe", 0, "invoicegnp.exe"},
		{"bidi isolates and marks", "a\u2066b\u2069c\u200fd", 0, "abcd"},
		{"zero width joiner is kept", "👨\u200d👩", 0, "👨\u200d👩"},
		{"8-bit csi", "a\u009b31mb", 0, "ab"},
		{"multi-byte", "Zürich – 東京 🚀", 0, "Zürich – 東京 🚀"},
		{"cut without splitting a rune", "ééééé", 3, "ééé"},
		{"cut emoji", "🚀🚀🚀🚀", 2, "🚀🚀"},
		{"cut drops trailing space", "ab cd", 3, "ab"},
		{"cut counts the collapsed space", "ab   cd", 4, "ab c"},
		{"short enough", "abc", 200, "abc"},
		{"empty", "", 10, ""},
	}
	for _, c := range cases {
		got := Clean(c.in, c.limit)
		if got != c.want {
			t.Errorf("%s: Clean(%q, %d) = %q, want %q", c.name, c.in, c.limit, got, c.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: result is not valid UTF-8", c.name)
		}
	}
}

func TestCleanCutsLongServerMessage(t *testing.T) {
	long := strings.Repeat("line of text\n", 60)
	got := Clean(long, 200)
	if n := len([]rune(got)); n > 200 {
		t.Fatalf("cleaned message has %d runes", n)
	}
	if strings.ContainsAny(got, "\n\r\t") {
		t.Fatalf("cleaned message still has line breaks: %q", got)
	}
}

type fakeState struct {
	entered  int
	restored int
}

func (f *fakeState) raw() (func(), error) {
	f.entered++
	return func() { f.restored++ }, nil
}

func readSecretFrom(input string) (secret string, out string, err error) {
	secret, out, _, err = readSecretWithState(input)
	return secret, out, err
}

func readSecretWithState(input string) (secret string, out string, state *fakeState, err error) {
	var buf bytes.Buffer
	state = &fakeState{}
	secret, err = readSecret(context.Background(), bufio.NewReader(strings.NewReader(input)), &buf, state.raw, "*", "Passphrase: ")
	return secret, buf.String(), state, err
}

func TestReadSecret(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantSecret string
		wantOut    string
		wantAbort  bool
	}{
		{"typing", "abc\r", "abc", "Passphrase: ***\r\n", false},
		{"backspace", "abcd\x7f\r", "abc", "Passphrase: ****\b \b\r\n", false},
		{"backspace key 0x08", "ab\x08\r", "a", "Passphrase: **\b \b\r\n", false},
		{"backspace on empty", "\x7fa\r", "a", "Passphrase: *\r\n", false},
		{"non-ascii", "pässword\r", "pässword", "Passphrase: ********\r\n", false},
		{"backspace erases a whole rune", "ä\x7f\r", "", "Passphrase: *\b \b\r\n", false},
		{"crlf is one enter", "abc\r\n", "abc", "Passphrase: ***\r\n", false},
		{"lf alone", "abc\n", "abc", "Passphrase: ***\r\n", false},
		{"ctrl-u clears", "abc\x15d\r", "d", "Passphrase: ***\b \b\b \b\b \b*\r\n", false},
		{"arrow keys ignored", "a\x1b[Ab\x1b[D\x1bOHc\r", "abc", "Passphrase: ***\r\n", false},
		{"delete key ignored", "ab\x1b[3~c\r", "abc", "Passphrase: ***\r\n", false},
		{"other control bytes ignored", "a\x00\x01b\r", "ab", "Passphrase: **\r\n", false},
		{"ctrl-d with text is ignored", "ab\x04c\r", "abc", "Passphrase: ***\r\n", false},
		{"empty secret", "\r", "", "Passphrase: \r\n", false},
		{"ctrl-c aborts", "ab\x03", "", "Passphrase: **\r\n", true},
		{"ctrl-d on empty aborts", "\x04", "", "Passphrase: \r\n", true},
		{"end of input aborts", "ab", "", "Passphrase: **\r\n", true},
	}
	for _, c := range cases {
		secret, out, err := readSecretFrom(c.input)
		if c.wantAbort != errors.Is(err, ErrAborted) {
			t.Errorf("%s: err = %v, want abort %v", c.name, err, c.wantAbort)
		}
		if !c.wantAbort && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if secret != c.wantSecret {
			t.Errorf("%s: secret = %q, want %q", c.name, secret, c.wantSecret)
		}
		if out != c.wantOut {
			t.Errorf("%s: output = %q, want %q", c.name, out, c.wantOut)
		}
	}
}

func TestReadSecretNeverEchoesTheText(t *testing.T) {
	_, out, err := readSecretFrom("correct horse\r")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "correct") || strings.Contains(out, "horse") {
		t.Fatalf("typed text leaked into the output: %q", out)
	}
	if got := strings.Count(out, "*"); got != len("correct horse") {
		t.Fatalf("mask count = %d", got)
	}
}

func TestReadSecretRestoresTheTerminalOnEveryPath(t *testing.T) {
	for _, input := range []string{"abc\r", "ab\x03", "\x04", "ab", "\r"} {
		_, _, state, _ := readSecretWithState(input)
		if state.entered != 1 || state.restored != 1 {
			t.Errorf("input %q: raw mode entered %d times, restored %d times", input, state.entered, state.restored)
		}
	}
}

func TestReadSecretReportsRawModeFailure(t *testing.T) {
	boom := errors.New("not a terminal")
	var out bytes.Buffer
	raw := func() (func(), error) { return nil, boom }
	if _, err := readSecret(context.Background(), bufio.NewReader(strings.NewReader("abc\r")), &out, raw, "*", "Passphrase: "); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the raw mode failure", err)
	}
	if out.Len() != 0 {
		t.Fatalf("nothing may be printed when raw mode fails, got %q", out.String())
	}
}

func TestReadSecretRestoresTheTerminalWhenCancelledMidRead(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	reader := pendingSecretReader{entered: entered, release: release, finished: finished}
	defer func() {
		close(release)
		<-finished
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	state := &fakeState{}
	done := make(chan error, 1)
	go func() {
		_, err := readSecret(ctx, bufio.NewReader(reader), io.Discard, state.raw, "*", "Passphrase: ")
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, ErrAborted) {
		t.Fatalf("err = %v, want ErrAborted", err)
	}
	if state.entered != 1 || state.restored != 1 {
		t.Fatalf("raw mode entered %d times, restored %d times", state.entered, state.restored)
	}
}

type pendingSecretReader struct {
	entered, release, finished chan struct{}
}

func (r pendingSecretReader) Read([]byte) (int, error) {
	close(r.entered)
	<-r.release
	close(r.finished)
	return 0, io.EOF
}
