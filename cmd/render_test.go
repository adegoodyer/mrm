package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

// N.7: offline, the summary says so once and lists each reason once, not
// the same error for every repo.
func TestSummaryOffline(t *testing.T) {
	var results []model.Result
	for i := range 40 {
		results = append(results, model.Result{
			Repo:    model.Repo{Name: fmt.Sprintf("repo-%02d", i)},
			Outcome: model.OutcomeFailed,
			Reason:  "fetch: can't resolve the remote host - offline?",
		})
	}
	var b bytes.Buffer
	(&app{}).printSummary(&b, "Synced", results, time.Second)
	out := b.String()
	if strings.Count(out, "offline?") != 2 || strings.Contains(out, "repo-") {
		t.Fatalf("want one offline line and one reason line, no repo names:\n%s", out)
	}
	if !strings.Contains(out, "40 repos  failed: fetch: can't resolve the remote host") {
		t.Fatalf("reason line missing its count:\n%s", out)
	}

	// Not offline when something worked: every failure is listed by name.
	results[0] = model.Result{Repo: model.Repo{Name: "ok"}, Outcome: model.OutcomeCurrent}
	b.Reset()
	(&app{}).printSummary(&b, "Synced", results, time.Second)
	if out := b.String(); strings.Contains(out, "every fetch failed") || !strings.Contains(out, "repo-39") {
		t.Fatalf("one repo fetched, so not offline:\n%s", out)
	}
}
