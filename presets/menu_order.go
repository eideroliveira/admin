package presets

import (
	"encoding/json"
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

var _ = json.Marshal

type MenuSearchItem struct {
	Key     string `json:"key"`
	Group   string `json:"group"`
	GroupUI string `json:"groupUI"`
	GroupEN string `json:"groupEN"`
	LabelUI string `json:"labelUI"`
	LabelEN string `json:"labelEN"`
	URI     string `json:"uri"`
}

type MenuOrderBuilder struct {
	p *Builder
	// string or *MenuGroupBuilder
	order             []interface{}
	modelMap          map[string]*ModelBuilder
	menuComponentFunc func(menus []h.HTMLComponent, menuGroupSelected, menuItemSelected string) h.HTMLComponent
	searchBoxEnabled  bool
	once              sync.Once
}

type menuOrderItem struct {
	groupName string
	model     *ModelBuilder
}

func NewMenuOrderBuilder(b *Builder) *MenuOrderBuilder {
	return &MenuOrderBuilder{p: b, searchBoxEnabled: true}
}

func (b *MenuOrderBuilder) SearchBox(v bool) *MenuOrderBuilder {
	b.searchBoxEnabled = v
	return b
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
	menus, searchItems := b.buildOrderedMenus(ctx, inOrderMap)
	unorderedMenus, unorderedSearchItems := b.buildUnorderedMenus(ctx, inOrderMap, b.referencedModelNames())

	menus = append(menus, unorderedMenus...)
	searchItems = append(searchItems, unorderedSearchItems...)

	activeMenuItem, selection := b.getActiveMenuState(ctx, inOrderMap)

	return b.buildMenuComponent(ctx, menus, activeMenuItem, selection, searchItems)
}

func (b *MenuOrderBuilder) initializeModelMap() {
	b.once.Do(func() {
		b.modelMap = make(map[string]*ModelBuilder)
		for _, m := range b.p.models {
			b.modelMap[m.uriName] = m
		}
	})
}

func (b *MenuOrderBuilder) buildOrderedMenus(ctx *web.EventContext, inOrderMap map[string]menuOrderItem) ([]h.HTMLComponent, []MenuSearchItem) {
	var menus []h.HTMLComponent
	var searchItems []MenuSearchItem

	for _, om := range b.order {
		switch v := om.(type) {
		case string:
			m, ok := b.check(v, "", ctx)
			if !ok {
				continue
			}
			menuItem := b.buildMenuItemWithModel(m, false, "", ctx, inOrderMap)
			if menuItem != nil {
				menus = append(menus, menuItem)
				labelEN := m.Info().Label()
				if m.label != "" && m.label != labelEN {
					labelEN = labelEN + " " + m.label
				}
				searchItems = append(searchItems, MenuSearchItem{
					Key:     m.uriName,
					Group:   "",
					GroupUI: "",
					GroupEN: "",
					LabelUI: m.Info().LabelName(ctx, false),
					LabelEN: labelEN,
					URI:     m.Info().URIName(),
				})
			}
		case *MenuGroupBuilder:
			menuGroup, groupSearchItems := b.buildMenuGroup(v, ctx, inOrderMap)
			if menuGroup != nil {
				menus = append(menus, menuGroup)
				searchItems = append(searchItems, groupSearchItems...)
			}
		default:
			panic(fmt.Sprintf("unknown menu order item type: %T\n", om))
		}
	}
	return menus, searchItems
}

func (b *MenuOrderBuilder) buildUnorderedMenus(ctx *web.EventContext, inOrderMap map[string]menuOrderItem, referenced map[string]struct{}) ([]h.HTMLComponent, []MenuSearchItem) {
	var menus []h.HTMLComponent
	var searchItems []MenuSearchItem

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
		menuItem := b.buildMenuItemWithModel(m, false, "", ctx, inOrderMap)
		if menuItem != nil {
			menus = append(menus, menuItem)
			labelEN := m.Info().Label()
			if m.label != "" && m.label != labelEN {
				labelEN = labelEN + " " + m.label
			}
			searchItems = append(searchItems, MenuSearchItem{
				Key:     m.uriName,
				Group:   "",
				GroupUI: "",
				GroupEN: "",
				LabelUI: m.Info().LabelName(ctx, false),
				LabelEN: labelEN,
				URI:     m.Info().URIName(),
			})
		}
	}
	return menus, searchItems
}

func (b *MenuOrderBuilder) buildMenuItem(name string, isSub bool, groupName string, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) h.HTMLComponent {
	m, ok := b.check(name, groupName, ctx)
	if !ok {
		return nil
	}
	return b.buildMenuItemWithModel(m, isSub, groupName, ctx, inOrderMap)
}

func (b *MenuOrderBuilder) buildMenuItemWithModel(m *ModelBuilder, isSub bool, groupName string, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) h.HTMLComponent {
	menuItem, err := m.menuItem(ctx, isSub)
	if err != nil {
		panic(err)
	}
	inOrderMap[m.uriName] = menuOrderItem{
		groupName: groupName,
		model:     m,
	}
	if b.searchBoxEnabled {
		if li, ok := menuItem.(*VListItemBuilder); ok {
			li.Attr(":style", fmt.Sprintf("locals.hiddenItems && locals.hiddenItems[%q] ? 'display: none !important' : ''", m.uriName))
			li.Attr("data-menu-item", m.uriName)
		}
	}
	return menuItem
}

func (b *MenuOrderBuilder) buildMenuGroup(v *MenuGroupBuilder, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) (h.HTMLComponent, []MenuSearchItem) {
	if !b.hasPermissionForMenuGroup(v, ctx) {
		return nil, nil
	}

	subMenus, subSearchItems := b.buildSubMenus(v, ctx, inOrderMap)
	if len(subMenus) == 0 {
		return nil, nil
	}

	activator := b.buildMenuGroupActivator(v, ctx)
	group := VListGroup(append([]h.HTMLComponent{activator}, subMenus...)...).Value(v.name)
	if b.searchBoxEnabled {
		group.Attr(":style", fmt.Sprintf("locals.hiddenGroups && locals.hiddenGroups[%q] ? 'display: none !important' : ''", v.name))
		group.Attr("data-menu-group", v.name)
	}
	return group, subSearchItems
}

func (b *MenuOrderBuilder) hasPermissionForMenuGroup(v *MenuGroupBuilder, ctx *web.EventContext) bool {
	return b.p.verifier.Do(PermList).SnakeOn("mg_"+v.name).WithReq(ctx.R).IsAllowed() == nil
}

func (b *MenuOrderBuilder) buildSubMenus(v *MenuGroupBuilder, ctx *web.EventContext, inOrderMap map[string]menuOrderItem) ([]h.HTMLComponent, []MenuSearchItem) {
	var subMenus []h.HTMLComponent
	var searchItems []MenuSearchItem
	groupUI := i18n.T(ctx.R, ModelsI18nModuleKey, v.name)
	for _, subItem := range v.subMenuItems {
		m, ok := b.check(subItem, v.name, ctx)
		if !ok {
			continue
		}
		menuItem := b.buildMenuItemWithModel(m, true, v.name, ctx, inOrderMap)
		if menuItem != nil {
			subMenus = append(subMenus, menuItem)
			labelEN := m.Info().Label()
			if m.label != "" && m.label != labelEN {
				labelEN = labelEN + " " + m.label
			}
			searchItems = append(searchItems, MenuSearchItem{
				Key:     m.uriName,
				Group:   v.name,
				GroupUI: groupUI,
				GroupEN: v.name,
				LabelUI: m.Info().LabelName(ctx, false),
				LabelEN: labelEN,
				URI:     m.Info().URIName(),
			})
		}
	}
	return subMenus, searchItems
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

const menuSearchScript = `<script>(function(){` +
	`if(window.gordpressMenuSearch)return;` +
	`function norm(s){if(!s)return'';return String(s).toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g,'').trim();}` +
	`window.gordpressMenuSearch=function(locals,query){` +
	`if(!locals)return;` +
	`var q=norm(query);` +
	`if(!q){` +
	`locals.hiddenGroups={};` +
	`locals.hiddenItems={};` +
	`locals.menuOpened=locals.defaultOpened?[locals.defaultOpened]:[];` +
	`locals.noMatches=false;` +
	`return;` +
	`}` +
	`var tokens=q.split(/\s+/).filter(Boolean);` +
	`var items=locals.items||[];` +
	`var hiddenItems={};` +
	`var groupMatchedItems={};` +
	`var groupMatchedTitle={};` +
	`var allGroups={};` +
	`var anyItemVisible=false;` +
	`for(var i=0;i<items.length;i++){` +
	`var it=items[i];` +
	`var grp=it.group||'';` +
	`if(grp){` +
	`allGroups[grp]=true;` +
	`if(!groupMatchedTitle[grp]){` +
	`var gUI=norm(it.groupUI);` +
	`var gEN=norm(it.groupEN);` +
	`var gm=tokens.every(function(t){return gUI.indexOf(t)!==-1||gEN.indexOf(t)!==-1;});` +
	`if(gm){groupMatchedTitle[grp]=true;}` +
	`}` +
	`}` +
	`}` +
	`for(var j=0;j<items.length;j++){` +
	`var item=items[j];` +
	`var g=item.group||'';` +
	`var lUI=norm(item.labelUI);` +
	`var lEN=norm(item.labelEN);` +
	`var uri=norm(item.uri);` +
	`var gUI=norm(item.groupUI);` +
	`var gEN=norm(item.groupEN);` +
	`var im=tokens.every(function(t){return lUI.indexOf(t)!==-1||lEN.indexOf(t)!==-1||uri.indexOf(t)!==-1||gUI.indexOf(t)!==-1||gEN.indexOf(t)!==-1;});` +
	`if(im){` +
	`if(g){groupMatchedItems[g]=(groupMatchedItems[g]||0)+1;}` +
	`anyItemVisible=true;` +
	`}else{` +
	`if(g&&groupMatchedTitle[g]){` +
	`anyItemVisible=true;` +
	`}else{` +
	`hiddenItems[item.key]=true;` +
	`}` +
	`}` +
	`}` +
	`var hiddenGroups={};` +
	`var openedGroups=[];` +
	`for(var grpName in allGroups){` +
	`if(groupMatchedItems[grpName]||groupMatchedTitle[grpName]){` +
	`openedGroups.push(grpName);` +
	`}else{` +
	`hiddenGroups[grpName]=true;` +
	`}` +
	`}` +
	`locals.hiddenItems=hiddenItems;` +
	`locals.hiddenGroups=hiddenGroups;` +
	`locals.menuOpened=openedGroups;` +
	`locals.noMatches=!anyItemVisible;` +
	`};` +
	`})();</script>`

func (b *MenuOrderBuilder) buildMenuComponent(ctx *web.EventContext, menus []h.HTMLComponent, menuGroupSelected, menuItemSelected string, searchItems []MenuSearchItem) h.HTMLComponent {
	if b.menuComponentFunc != nil {
		return b.menuComponentFunc(menus, menuGroupSelected, menuItemSelected)
	}

	if !b.searchBoxEnabled || len(searchItems) == 0 {
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

	msgr := MustGetMessages(ctx.R)
	placeholder := msgr.SearchMenu
	if placeholder == "" {
		if strings.HasPrefix(strings.ToLower(msgr.Search), "busc") {
			placeholder = "Buscar menu..."
		} else {
			placeholder = "Search menu..."
		}
	}
	noMatchText := msgr.NoMatchingMenus
	if noMatchText == "" {
		if strings.HasPrefix(strings.ToLower(msgr.Search), "busc") {
			noMatchText = "Nenhum menu encontrado"
		} else {
			noMatchText = "No matching menus"
		}
	}

	searchField := VTextField().
		Placeholder(placeholder).
		Density(DensityCompact).
		Variant(VariantOutlined).
		PrependInnerIcon("mdi-magnify").
		HideDetails(true).
		Clearable(true).
		Class("menu-search-box mb-2").
		Attr("v-model", "locals.menuSearch").
		Attr("@update:model-value", "window.gordpressMenuSearch(locals, $event)").
		Attr("@click:clear", "window.gordpressMenuSearch(locals, '')").
		Attr("@keydown.esc", "locals.menuSearch = ''; window.gordpressMenuSearch(locals, '')")

	noMatchComp := h.Div(
		VIcon("mdi-magnify").Size(SizeSmall).Class("mb-1 text-medium-emphasis"),
		h.Div().Text(noMatchText).Class("text-caption text-medium-emphasis"),
	).Class("text-center py-4").Attr("v-if", "locals.noMatches")

	if ctx.Injector != nil {
		ctx.Injector.TailHTMLComponent("gordpressMenuSearch", h.RawHTML(menuSearchScript), true)
	}

	itemsJSON, _ := json.Marshal(searchItems)
	openedInit := "[]"
	if menuGroupSelected != "" {
		openedInit = fmt.Sprintf("[%q]", menuGroupSelected)
	}
	initScript := fmt.Sprintf(`{ menuOpened: %s, defaultOpened: %q, selection: [%q], menuSearch: "", hiddenGroups: {}, hiddenItems: {}, noMatches: false, items: %s }`,
		openedInit, menuGroupSelected, menuItemSelected, string(itemsJSON))

	return h.Div(
		web.Scope(
			h.Div(
				searchField,
				VList(menus...).
					Attr(":open-strategy", "locals.menuSearch ? 'multiple' : 'single'").
					Class("primary--text").
					Density(DensityCompact).
					Attr("v-model:opened", "locals.menuOpened").
					Attr("v-model:selected", "locals.selection").
					Attr("color", "transparent"),
				noMatchComp,
				h.If(ctx.Injector == nil, h.RawHTML(menuSearchScript)),
			),
		).VSlot("{ locals }").Init(initScript),
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
