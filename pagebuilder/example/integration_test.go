package example_test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/qor/qor5/pagebuilder/example"
	"github.com/theplant/gofixtures"
)

func TestEditor(t *testing.T) {
	db := example.ConnectDB()
	sdb, _ := db.DB()
	gofixtures.Data(
		gofixtures.Sql(`
INSERT INTO public.page_builder_pages (id, title, slug) VALUES (1, '123', '123');
INSERT INTO public.page_builder_containers (id, page_id, name, model_type, model_id, "order") VALUES (1, 1, 'text_and_image', '123', 1, 0);
INSERT INTO public.text_and_images (text, image, id) VALUES ('Hello Text and Image', null, 1);
`, []string{"page_builder_pages", "page_builder_containers", "text_and_images"}),
	).TruncatePut(sdb)

	pb := example.ConfigPageBuilder(db)

	r := httptest.NewRequest("GET", "/page_builder/editors/123", nil)
	w := httptest.NewRecorder()
	pb.ServeHTTP(w, r)
	fmt.Println(w.Body.String())
}
