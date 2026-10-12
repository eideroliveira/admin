package presets

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qor5/web/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	h "github.com/theplant/htmlgo"
)

type menuOrderTestCustomer struct{ ID uint }
type menuOrderTestOrder struct{ ID uint }

func TestMenuOrderSearchBox_Rendered(t *testing.T) {
	pb := New()
	pb.Model(&menuOrderTestCustomer{}).Label("Customers")
	pb.Model(&menuOrderTestOrder{}).Label("Orders")
	pb.MenuOrder(
		pb.MenuGroup("E-Commerce").SubItems("menu-order-test-customers", "menu-order-test-orders").Icon("mdi-cart"),
	)

	ctx := &web.EventContext{R: httptest.NewRequest("GET", "/admin", http.NoBody)}
	comp := pb.menuOrder.CreateMenus(ctx)

	var buf bytes.Buffer
	require.NoError(t, h.Fprint(&buf, comp, context.Background()))
	html := buf.String()

	assert.Contains(t, html, "menu-search-box", "search text field should be rendered")
	assert.Contains(t, html, "mdi-magnify", "search icon should be present")
	assert.Contains(t, html, "Search menu...", "default English placeholder should be rendered")
	assert.Contains(t, html, `data-menu-group='E-Commerce'`, "menu group should have data-menu-group attribute")
	assert.Contains(t, html, `data-menu-item='menu-order-test-customers'`, "menu item should have data-menu-item attribute")
	assert.Contains(t, html, `data-menu-item='menu-order-test-orders'`, "menu item should have data-menu-item attribute")
	assert.Contains(t, html, "locals.hiddenGroups", "menu group should bind hiddenGroups style")
	assert.Contains(t, html, "locals.hiddenItems", "menu item should bind hiddenItems style")
	assert.Contains(t, html, "window.gordpressMenuSearch", "gordpressMenuSearch handler should be bound")
	assert.Contains(t, html, `"labelEN":"Customers"`, "search index should contain English label")
	assert.Contains(t, html, `"groupEN":"E-Commerce"`, "search index should contain English group")
}

func TestMenuOrderSearchBox_CanBeDisabled(t *testing.T) {
	pb := New()
	pb.Model(&menuOrderTestCustomer{}).Label("Customers")
	pb.MenuOrder(
		pb.MenuGroup("E-Commerce").SubItems("menu-order-test-customers"),
	)
	pb.GetMenuOrder().SearchBox(false)

	ctx := &web.EventContext{R: httptest.NewRequest("GET", "/admin", http.NoBody)}
	comp := pb.menuOrder.CreateMenus(ctx)

	var buf bytes.Buffer
	require.NoError(t, h.Fprint(&buf, comp, context.Background()))
	html := buf.String()

	assert.NotContains(t, html, "menu-search-box", "search text field should not be rendered when disabled")
}
