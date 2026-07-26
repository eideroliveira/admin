package worker

import (
	"context"
	"strings"
	"testing"
)

// TestProgressTextDisplayEscapes is the regression for stored XSS in the job
// detail view and the action-job dialog, which both render a job instance's
// progress text. Handlers build that text out of data they do not control —
// a file name inside an uploaded package, a message from a remote API — so
// rendering it with RawHTML (as this did) turned any such name into script
// running in the browser of whichever operator was watching the run. The v-pre
// wrapper is not a defence: it only stops Vue from compiling mustaches.
func TestProgressTextDisplayEscapes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		progressText string
		wantNot      string
		want         string
	}{
		{
			name:         "file name from the source package",
			progressText: `3/12 arquivos — último: licao<img src=x onerror=alert(1)>.pdf`,
			wantNot:      "<img",
			want:         "licao&lt;img src=x onerror=alert(1)&gt;.pdf",
		},
		{
			name:         "script tag",
			progressText: `<script>alert(1)</script>`,
			wantNot:      "<script",
			want:         "&lt;script&gt;",
		},
		{
			name:         "attribute break-out",
			progressText: `a" onmouseover="alert(1)`,
			wantNot:      `onmouseover="alert(1)`,
			want:         "&#34;",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := progressTextDisplay(tc.progressText).MarshalHTML(context.Background())
			if err != nil {
				t.Fatalf("marshal html: %v", err)
			}
			got := string(out)

			if strings.Contains(got, tc.wantNot) {
				t.Fatalf("progress text reached the page unescaped (found %q):\n%s", tc.wantNot, got)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected escaped %q in output:\n%s", tc.want, got)
			}
		})
	}
}

// TestJobLogLineIsInert covers the other half of the same data path: job logs
// carry the same uncontrolled names the progress text does. Escaping alone is
// not enough here, because go-plaid compiles the response body as a Vue
// template — three of the four log render sites were missing v-pre, so a
// mustache in a log line was evaluated as an expression rather than shown.
func TestJobLogLineIsInert(t *testing.T) {
	for _, tc := range []struct {
		name    string
		log     string
		wantNot string
	}{
		{
			name:    "html is escaped",
			log:     `traduzindo licao<img src=x onerror=alert(1)>.pdf`,
			wantNot: "<img",
		},
		{
			name:    "vue expression is not evaluated",
			log:     `traduzindo {{constructor.constructor('alert(1)')()}}.pdf`,
			wantNot: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := jobLogLine(tc.log).MarshalHTML(context.Background())
			if err != nil {
				t.Fatalf("marshal html: %v", err)
			}
			got := string(out)

			if tc.wantNot != "" && strings.Contains(got, tc.wantNot) {
				t.Fatalf("log line reached the page unescaped (found %q):\n%s", tc.wantNot, got)
			}
			// v-pre is what stops Vue from compiling a mustache in the line.
			if !strings.Contains(got, "v-pre") {
				t.Fatalf("log line rendered without v-pre — Vue will evaluate {{ }} in it:\n%s", got)
			}
		})
	}
}

// TestProgressTextDisplayEmpty keeps the "no progress text, no box" behaviour
// the render sites relied on before the helper was extracted.
func TestProgressTextDisplayEmpty(t *testing.T) {
	out, err := progressTextDisplay("").MarshalHTML(context.Background())
	if err != nil {
		t.Fatalf("marshal html: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected nothing rendered for empty progress text, got %q", out)
	}
}
