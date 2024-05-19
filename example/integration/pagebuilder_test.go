package integration_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qor5/admin/v3/example/admin"
	"github.com/qor5/admin/v3/note"
	"github.com/qor5/admin/v3/pagebuilder"
	. "github.com/qor5/web/v3/multipartestutils"
	"github.com/theplant/gofixtures"
	"github.com/theplant/testenv"
	"gorm.io/gorm"
)

var TestDB *gorm.DB

func TestMain(m *testing.M) {
	env, err := testenv.New().DBEnable(true).SetUp()
	if err != nil {
		panic(err)
	}
	defer env.TearDown()
	TestDB = env.DB
	m.Run()
}

var pageBuilderData = gofixtures.Data(gofixtures.Sql(`
INSERT INTO public.page_builder_categories (id, created_at, updated_at, deleted_at, name, path, description, locale_code) VALUES (1, '2024-05-17 15:25:31.134801 +00:00', '2024-05-17 15:25:31.134801 +00:00', null, '123', '/12', '', 'International');

INSERT INTO public.page_builder_pages (id, created_at, updated_at, deleted_at, title, slug, category_id, status, online_url, scheduled_start_at, scheduled_end_at, actual_start_at, actual_end_at, version, version_name, parent_version, locale_code, seo) VALUES (1, '2024-05-17 15:25:39.716658 +00:00', '2024-05-17 15:25:39.716658 +00:00', null, '12312', '/123', 1, 'draft', '', null, null, null, null, '2024-05-18-v01', '2024-05-18-v01', '', 'International', '{"OpenGraphImageFromMediaLibrary":{"ID":0,"Url":"","VideoLink":"","FileName":"","Description":""}}');
`, []string{"page_builder_pages", "page_builder_categories"}))

func TestPageBuilder(t *testing.T) {
	h := admin.TestHandler(TestDB)
	dbr, _ := TestDB.DB()

	cases := []TestCase{
		{
			Name:  "Index Page",
			Debug: true,
			ReqFunc: func() *http.Request {
				pageBuilderData.TruncatePut(dbr)
				return httptest.NewRequest("GET", "/pages", nil)
			},
			ExpectPageBodyContains: []string{"12312"},
		},

		{
			Name:  "Page Builder Detail Page",
			Debug: true,
			ReqFunc: func() *http.Request {
				pageBuilderData.TruncatePut(dbr)
				return httptest.NewRequest("GET", "/pages/1_2024-05-18-v01_International", nil)
			},
			ExpectPageBodyContains: []string{
				"createNoteDialogEvent",
				`url("/pages")`,
				`"1_2024-05-18-v01_International"`,
			},
		},

		{
			Name:  "Page Builder Editor Show Add Notes Dialog",
			Debug: true,
			ReqFunc: func() *http.Request {
				pageBuilderData.TruncatePut(dbr)
				req := NewMultipartBuilder().
					PageURL("/pages?__execute_event__=createNoteDialogEvent&id=1_2024-05-18-v01_International&overlay=dialog").
					BuildEventFuncRequest()
				return req
			},
			ExpectPortalUpdate0Contains: []string{`eventFunc("createNoteEvent")`},
		},
		{
			Name:  "Page Builder Editor Add a Note",
			Debug: true,
			ReqFunc: func() *http.Request {
				pageBuilderData.TruncatePut(dbr)
				req := NewMultipartBuilder().
					PageURL("/pages?__execute_event__=createNoteEvent&overlay=dialog&resource_id=1_2024-05-18-v01_International&resource_type=Pages").
					AddField("Content", "Hello").
					BuildEventFuncRequest()

				return req
			},
			EventResponseMatch: func(t *testing.T, er *TestEventResponse) {
				n := note.QorNote{}
				TestDB.Find(&n)
				if n.Content != "Hello" {
					t.Error("Note not created", n)
				}
			},
		},
		{
			Name:  "Page Builder Editor Duplicate A Page",
			Debug: true,
			ReqFunc: func() *http.Request {
				pageBuilderData.TruncatePut(dbr)
				req := NewMultipartBuilder().
					PageURL("/pages/1_2024-05-18-v01_International?__execute_event__=publish_EventDuplicateVersion").
					BuildEventFuncRequest()

				return req
			},
			EventResponseMatch: func(t *testing.T, er *TestEventResponse) {
				var pages []*pagebuilder.Page
				TestDB.Find(&pages)
				if len(pages) != 2 {
					t.Error("Page not duplicated", pages)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			RunCase(t, c, h)
		})
	}
}
