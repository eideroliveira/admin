package worker

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qor5/web/v3"
	"github.com/qor5/x/v3/perm"
)

func jobListHTML(t *testing.T, b *Builder, form url.Values) string {
	t.Helper()
	r := httptest.NewRequest("POST", "/workers", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := &web.EventContext{R: r}
	out, err := b.jobSelectList(ctx, "").MarshalHTML(r.Context())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return string(out)
}

// TestJobListFilterPicksEachListsJobs: the filter sees the list the drawer
// was opened for, decides what it shows, and the list rides the drawer's form
// so the next event (going back from a picked job) asks for the same list.
func TestJobListFilterPicksEachListsJobs(t *testing.T) {
	permVerifier = perm.NewVerifier("workers", nil)
	b := &Builder{}
	for _, n := range []string{"Alpha", "Beta", "Gamma"} {
		b.jbs = append(b.jbs, newJob(b, n))
	}

	var gotList string
	b.JobListFilter(func(_ *web.EventContext, list string, names []string) []string {
		gotList = list
		if list == "other" {
			return []string{"Gamma"}
		}
		return []string{"Alpha", "Beta"}
	})

	main := jobListHTML(t, b, url.Values{})
	if gotList != "" || !strings.Contains(main, "Alpha") || !strings.Contains(main, "Beta") || strings.Contains(main, "Gamma") {
		t.Fatalf("New list (%q) = %s", gotList, main)
	}

	other := jobListHTML(t, b, url.Values{ParamJobList: {"other"}})
	if gotList != "other" || strings.Contains(other, "Alpha") || !strings.Contains(other, "Gamma") {
		t.Fatalf("other list (%q) = %s", gotList, other)
	}
	if !strings.Contains(other, ParamJobList) || !strings.Contains(other, `"other"`) {
		t.Fatalf("the list is not carried in the drawer's form: %s", other)
	}
}

// TestJobListWithoutFilterListsEveryGlobalJob: no filter keeps the old list.
func TestJobListWithoutFilterListsEveryGlobalJob(t *testing.T) {
	permVerifier = perm.NewVerifier("workers", nil)
	b := &Builder{}
	b.jbs = append(b.jbs, newJob(b, "Alpha"), newJob(b, "Beta"))
	hidden := newJob(b, "Hidden")
	hidden.global = false
	b.jbs = append(b.jbs, hidden)

	out := jobListHTML(t, b, url.Values{})
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") || strings.Contains(out, "Hidden") {
		t.Fatalf("list = %s", out)
	}
}
