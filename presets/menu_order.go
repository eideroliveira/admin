package presets

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/jinzhu/inflection"
	"github.com/qor5/web/v3"
	"github.com/qor5/x/v3/i18n"
	. "github.com/qor5/x/v3/ui/vuetify"
	h "github.com/theplant/htmlgo"
)

type MenuOrderBuilder struct {
	p *Builder
	// string or *MenuGroupBuilder
	order []interface{}

	modelMap map[string]*ModelBuilder
}

func newMenuOrderBuilder(b *Builder) *MenuOrderBuilder {
	return &MenuOrderBuilder{p: b}
}

func (b *MenuOrderBuilder) isMenuGroupInOrder(mgb *MenuGroupBuilder) bool {
	for _, v := range b.order {
		if v == mgb {
			return true
		}
	}
	return false
}

func (b *MenuOrderBuilder) removeMenuGroupInOrder(mgb *MenuGroupBuilder) {
	for i, om := range b.order {
		if om == mgb {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
}

func (b *MenuOrderBuilder) Append(items ...interface{}) {
	for _, item := range items {
		switch v := item.(type) {
		case string:
			b.order = append(b.order, v)
		case *MenuGroupBuilder:
			if b.isMenuGroupInOrder(v) {
				b.removeMenuGroupInOrder(v)
			}
			b.order = append(b.order, v)
		default:
			panic(fmt.Sprintf("unknown menu order item type: %T\n", item))
		}
	}
}

func (b *MenuOrderBuilder) check(item string, ctx *web.EventContext) (*ModelBuilder, bool) {
	if b.modelMap == nil {
		b.modelMap = make(map[string]*ModelBuilder)
		for _, m := range b.p.models {
			b.modelMap[m.uriName] = m
		}
	}
	m, ok := b.modelMap[item]
	if !ok {
		m, ok = b.modelMap[inflection.Plural(strcase.ToKebab(item))]
	}
	if !ok {
		return nil, false
	}
	disabled := m.notInMenu || (m.Info().Verifier().Do(PermList).WithReq(ctx.R).IsAllowed() != nil)
	if disabled {
		return m, false
	}
	return m, true
}

func (b *MenuOrderBuilder) CreateMenus(ctx *web.EventContext) (r h.HTMLComponent) {
	b.modelMap = make(map[string]*ModelBuilder)
	for _, m := range b.p.models {
		b.modelMap[m.uriName] = m
	}
	var (
		activeMenuItem string
		selection      string
	)
	inOrderMap := make(map[string]struct{})
	var menus []h.HTMLComponent
	for _, om := range b.order {
		if v, ok := om.(string); ok {
			m, menuItem := b.menuItem(v, false, ctx)
			if menuItem == nil {
				continue
			}
			menus = append(menus, menuItem)
			inOrderMap[m.uriName] = struct{}{}

			if b.isMenuItemActive(m, ctx) {
				selection = m.label
			}
			continue
		}

		v := om.(*MenuGroupBuilder)
		if b.p.verifier.Do(PermList).SnakeOn("mg_"+v.name).WithReq(ctx.R).IsAllowed() != nil {
			continue
		}
		groupIcon := v.icon
		if groupIcon == "" {
			groupIcon = defaultMenuIcon(v.name)
		}
		subMenus := []h.HTMLComponent{
			h.Template(
				VListItem(
					web.Slot(
						VIcon(groupIcon),
					).Name("prepend"),
					VListItemTitle().Attr("style", fmt.Sprintf("white-space: normal; font-weight: %s;font-size: 14px;", menuFontWeight)),
					// VListItemTitle(h.Text(i18n.T(ctx.R, ModelsI18nModuleKey, v.name))).
				).Attr("v-bind", "props").
					Title(i18n.T(ctx.R, ModelsI18nModuleKey, v.name)).
					Class("rounded-lg"),
				// Value(i18n.T(ctx.R, ModelsI18nModuleKey, v.name)),
			).Attr("v-slot:activator", "{ props }"),
		}
		subCount := 0
		for _, subOm := range v.subMenuItems {
			m, menuItem := b.menuItem(subOm, true, ctx)
			if m != nil {
				m.menuGroupName = v.name
			}
			if menuItem == nil {
				continue
			}
			subMenus = append(subMenus, menuItem)
			subCount++
			inOrderMap[m.uriName] = struct{}{}
			if b.isMenuItemActive(m, ctx) {
				activeMenuItem = v.name
				selection = m.label
			}
		}
		if subCount == 0 {
			continue
		}

		menus = append(menus,
			VListGroup(subMenus...).Value(v.name),
		)
	}

	for _, m := range b.p.models {
		m, menuItem := b.menuItem(m.uriName, false, ctx)
		if menuItem == nil {
			continue
		}
		if b.isMenuItemActive(m, ctx) {
			selection = m.label
		}
		menus = append(menus, menuItem)
	}

	r = h.Div(
		web.Scope(
			VList(menus...).
				OpenStrategy("single").
				Class("primary--text").
				Density(DensityCompact).
				Attr("v-model:opened", "locals.menuOpened").
				Attr("v-model:selected", "locals.selection").
				Attr("color", "transparent"),
			// .Attr("v-model:selected", h.JSONString([]string{"Pages"})),
		).VSlot("{ locals }").Init(
			fmt.Sprintf(`{ menuOpened:  [%q]}`, activeMenuItem),
			fmt.Sprintf(`{ selection:  [%q]}`, selection),
		))
	return
}

func (b *MenuOrderBuilder) menuItem(name string, isSub bool, ctx *web.EventContext) (*ModelBuilder, h.HTMLComponent) {
	m, ok := b.check(name, ctx)
	if !ok {
		return m, nil
	}

	menuItem, err := m.menuItem(ctx, isSub)
	if err != nil {
		panic(err)
	}
	return m, menuItem
}

func (b *MenuOrderBuilder) isMenuItemActive(m *ModelBuilder, ctx *web.EventContext) bool {
	href := m.Info().ListingHref()
	if m.link != "" {
		href = m.link
	}
	path := strings.TrimSuffix(ctx.R.URL.Path, "/")
	if path == "" && href == "/" {
		return true
	}
	if path == href {
		return true
	}
	if href == b.p.prefix {
		return false
	}
	if href != "/" && strings.HasPrefix(path, href) {
		return true
	}

	return false
}
