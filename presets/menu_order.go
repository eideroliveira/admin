package presets

import (
	"fmt"
	"strings"
	"sync"

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
	order             []interface{}
	modelMap          map[string]*ModelBuilder
	menuComponentFunc func(menus []h.HTMLComponent, menuGroupSelected, menuItemSelected string) h.HTMLComponent
	once              sync.Once
}

type menuOrderItem struct {
	groupName string
	model     *ModelBuilder
}

func NewMenuOrderBuilder(b *Builder) *MenuOrderBuilder {
	return &MenuOrderBuilder{p: b}
}

func (b *MenuOrderBuilder) MenuComponentFunc(fn func(menus []h.HTMLComponent, menuGroupSelected, menuItemSelected string) h.HTMLComponent) *MenuOrderBuilder {
	b.menuComponentFunc = fn
	return b
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

// resolveModel maps a menu-order name to its registered model, applying the
// same normalization as check (exact match, then pluralized-kebab fallback)
// but WITHOUT the permission / notInMenu gate. Used to know which models are
// referenced by MenuOrder regardless of whether the current user may see them.
func (b *MenuOrderBuilder) resolveModel(item string) (*ModelBuilder, bool) {
	m, ok := b.modelMap[item]
	if !ok {
		m, ok = b.modelMap[inflection.Plural(strcase.ToKebab(item))]
	}
	return m, ok
}

func (b *MenuOrderBuilder) check(item string, groupName string, ctx *web.EventContext) (*ModelBuilder, bool) {
	m, ok := b.resolveModel(item)
	if !ok {
		return nil, false
	}
	disabled := m.notInMenu || (m.Info().Verifier().Do(PermList).WithReq(ctx.R).IsAllowed() != nil)
	if disabled {
		return m, false
	}
	return m, true
}

// referencedModelNames collects the uriName of every model referenced by
// MenuOrder — both top-level string items and menu-group subitems — regardless
// of permissions. buildUnorderedMenus uses this so that a model whose group is
// hidden (e.g. the user lacks the mg_<group> permission) is not re-surfaced as
// a flat top-level item. Only models never mentioned in MenuOrder at all are
// treated as genuinely "unordered".
func (b *MenuOrderBuilder) referencedModelNames() map[string]struct{} {
	referenced := make(map[string]struct{})
	mark := func(name string) {
		if m, ok := b.resolveModel(name); ok {
			referenced[m.uriName] = struct{}{}
		}
	}
	for _, om := range b.order {
		switch v := om.(type) {
		case string:
			mark(v)
		case *MenuGroupBuilder:
			for _, sub := range v.subMenuItems {
				mark(sub)
			}
		}
	}
	return referenced
}

func (b *MenuOrderBuilder) CreateMenus(ctx *web.EventContext) h.HTMLComponent {
	b.initializeModelMap()

	inOrderMap := make(map[string]menuOrderItem)
	menus := b.buildOrderedMenus(ctx, inOrderMap)
	unorderedMenus := b.buildUnorderedMenus(ctx, inOrderMap, b.referencedModelNames())

	menus = append(menus, unorderedMenus...)

	activeMenuItem, selection := b.getActiveMenuState(ctx, inOrderMap)

	return b.buildMenuComponent(menus, activeMenuItem, selection)
}

func (b *MenuOrderBuilder) initializeModelMap() {
	b.once.Do(func() {
		b.modelMap = make(map[string]*ModelBuilder)
		for _, m := range b.p.models {
			b.modelMap[m.uriName] = m
		}
	})
}

func (b *MenuOrderBuilder) buildOrderedMenus(ctx *web.EventContext, inOrderMap map[string]menuOrderItem) []h.HTMLComponent {
	var menus []h.HTMLComponent

	for _, om := range b.order {
		switch v := om.(type) {
		case string:
			menuItem := b.buildMenuItem(v, false, "", ctx, inOrderMap)
			if menuItem != nil {
				menus = append(menus, menuItem)
			}
		case *MenuGroupBuilder:
			menuGroup := b.buildMenuGroup(v, ctx, inOrderMap)
			if menuGroup != nil {
				menus = append(menus, menuGroup)
			}
		default:
			panic(fmt.Sprintf("unknown menu order item type: %T\n", om))
		}
	}
	return menus
}

func (b *MenuOrderBuilder) buildUnorderedMenus(ctx *web.EventContext, inOrderMap map[string]menuOrderItem, referenced map[string]struct{}) []h.HTMLComponent {
	var menus []h.HTMLComponent

	for _, m := range b.p.models {
		// Skip any model referenced by MenuOrder — even when its group was
		// suppressed (permission-denied or empty), it must not leak out as a
		// flat top-level item. Only models never listed in MenuOrder are
		// surfaced here.
		if _, exists := referenced[m.uriName]; exists {
			continue
		}
		if _, exists := inOrderMap[m.uriName]; exists {
			continue
		}
		menuItem := b.buildMenuItem(m.uriName, false, "", ctx, inOrderMap)
		if menuItem != nil {
			menus = append(menus, menuItem)
		}
	}
	return menus
}

func (b *MenuOrderBuilder) buildMenuItem(name string, isSub bool, groupName string, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) h.HTMLComponent {
	m, ok := b.check(name, groupName, ctx)
	if !ok {
		return nil
	}
	menuItem, err := m.menuItem(ctx, isSub)
	if err != nil {
		panic(err)
	}
	inOrderMap[m.uriName] = menuOrderItem{
		groupName: groupName,
		model:     m,
	}
	return menuItem
}

func (b *MenuOrderBuilder) buildMenuGroup(v *MenuGroupBuilder, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) h.HTMLComponent {
	if !b.hasPermissionForMenuGroup(v, ctx) {
		return nil
	}

	subMenus := b.buildSubMenus(v, ctx, inOrderMap)
	if len(subMenus) == 0 {
		return nil
	}

	activator := b.buildMenuGroupActivator(v, ctx)
	return VListGroup(append([]h.HTMLComponent{activator}, subMenus...)...).Value(v.name)
}

func (b *MenuOrderBuilder) hasPermissionForMenuGroup(v *MenuGroupBuilder, ctx *web.EventContext) bool {
	return b.p.verifier.Do(PermList).SnakeOn("mg_"+v.name).WithReq(ctx.R).IsAllowed() == nil
}

func (b *MenuOrderBuilder) buildSubMenus(v *MenuGroupBuilder, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) []h.HTMLComponent {
	var subMenus []h.HTMLComponent
	for _, subItem := range v.subMenuItems {
		menuItem := b.buildMenuItem(subItem, true, v.name, ctx, inOrderMap)
		if menuItem != nil {
			subMenus = append(subMenus, menuItem)
		}
	}
	return subMenus
}

func (b *MenuOrderBuilder) buildMenuGroupActivator(v *MenuGroupBuilder, ctx *web.EventContext) h.HTMLComponent {
	return h.Template(
		VListItem(
			web.Slot(VIcon(v.icon)).Name("prepend"),
			VListItemTitle().
				Attr("style", fmt.Sprintf("white-space: normal; font-weight: %s; font-size: 14px;", menuFontWeight)),
		).
			Attr("v-bind", "props").
			Title(i18n.T(ctx.R, ModelsI18nModuleKey, v.name)).
			Class("rounded-lg"),
	).Attr("v-slot:activator", "{ props }")
}

func (b *MenuOrderBuilder) getActiveMenuState(ctx *web.EventContext, inOrderMap map[string]menuOrderItem) (menuGroupSelected, menuItemSelected string) {
	for _, v := range inOrderMap {
		if b.isMenuItemActive(v.model, ctx) {
			menuGroupSelected = v.groupName
			menuItemSelected = v.model.label
		}
	}
	return
}

func (b *MenuOrderBuilder) buildMenuComponent(menus []h.HTMLComponent, menuGroupSelected, menuItemSelected string) h.HTMLComponent {
	if b.menuComponentFunc != nil {
		return b.menuComponentFunc(menus, menuGroupSelected, menuItemSelected)
	}
	return h.Div(
		web.Scope(
			VList(menus...).
				OpenStrategy("single").
				Class("primary--text").
				Density(DensityCompact).
				Attr("v-model:opened", "locals.menuOpened").
				Attr("v-model:selected", "locals.selection").
				Attr("color", "transparent"),
		).VSlot("{ locals }").Init(
			fmt.Sprintf(`{ menuOpened: [%q] }`, menuGroupSelected),
			fmt.Sprintf(`{ selection: [%q] }`, menuItemSelected),
		),
	)
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
