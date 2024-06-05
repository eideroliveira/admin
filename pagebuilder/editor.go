package pagebuilder

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	vx "github.com/qor5/ui/v3/vuetifyx"

	"github.com/qor5/admin/v3/l10n"
	"github.com/qor5/admin/v3/presets"
	"github.com/qor5/admin/v3/presets/actions"
	"github.com/qor5/admin/v3/publish"
	. "github.com/qor5/ui/v3/vuetify"
	"github.com/qor5/web/v3"
	"github.com/sunfmin/reflectutils"
	h "github.com/theplant/htmlgo"
	"gorm.io/gorm"
)

const (
	AddContainerEvent                = "page_builder_AddContainerEvent"
	DeleteContainerConfirmationEvent = "page_builder_DeleteContainerConfirmationEvent"
	DeleteContainerEvent             = "page_builder_DeleteContainerEvent"
	MoveContainerEvent               = "page_builder_MoveContainerEvent"
	MoveUpDownContainerEvent         = "page_builder_MoveUpDownContainerEvent"
	ToggleContainerVisibilityEvent   = "page_builder_ToggleContainerVisibilityEvent"
	MarkAsSharedContainerEvent       = "page_builder_MarkAsSharedContainerEvent"
	RenameContainerDialogEvent       = "page_builder_RenameContainerDialogEvent"
	RenameContainerEvent             = "page_builder_RenameContainerEvent"
	ShowSortedContainerDrawerEvent   = "page_builder_ShowSortedContainerDrawerEvent"
	ReloadRenderPageOrTemplateEvent  = "page_builder_ReloadRenderPageOrTemplateEvent"
	AutoSaveContainerEvent           = "page_builder_AutoSaveContainerEvent"

	paramPageID          = "pageID"
	paramPageVersion     = "pageVersion"
	paramLocale          = "locale"
	paramStatus          = "status"
	paramContainerID     = "containerID"
	paramContainerDataID = "containerDataID"
	paramMoveResult      = "moveResult"
	paramContainerName   = "containerName"
	paramSharedContainer = "sharedContainer"
	paramModelID         = "modelID"
	paramModelName       = "modelName"
	paramMoveDirection   = "moveDirection"
	paramsTpl            = "tpl"
	paramsDevice         = "device"
	paramsDisplayName    = "DisplayName"
	paramTab             = "tab"

	DevicePhone    = "phone"
	DeviceTablet   = "tablet"
	DeviceComputer = "computer"

	EventUp          = "up"
	EventDown        = "down"
	EventDelete      = "delete"
	EventAdd         = "add"
	EventEdit        = "edit"
	iframeHeightName = "_iframeHeight"

	EditorTabElements = "Elements"
	EditorTabLayers   = "Layers"
)

const editorPreviewContentPortal = "editorPreviewContentPortal"

func (b *Builder) Editor(m *ModelBuilder) web.PageFunc {
	return func(ctx *web.EventContext) (r web.PageResponse, err error) {
		var (
			deviceToggler, versionComponent      h.HTMLComponent
			editContainerDrawer, navigatorDrawer h.HTMLComponent
			tabContent                           web.PageResponse
			activeDevice                         int
			pageAppbarContent                    []h.HTMLComponent
			exitHref                             string

			device          = ctx.R.FormValue(paramsDevice)
			containerDataID = ctx.R.FormValue(paramContainerDataID)
			obj             = m.mb.NewModel()
		)

		switch device {
		case DeviceTablet:
			activeDevice = 1
		case DevicePhone:
			activeDevice = 2
		}

		if containerDataID != "" {
			arr := strings.Split(containerDataID, "_")
			if len(arr) >= 2 {
				editEvent := web.GET().EventFunc(actions.Edit).
					URL(fmt.Sprintf(`%s/%s`, b.prefix, arr[0])).
					Query(presets.ParamID, arr[1]).
					Query(presets.ParamPortalName, pageBuilderRightContentPortal).
					Query(presets.ParamOverlay, actions.Content).Go()
				editContainerDrawer = web.RunScript(fmt.Sprintf(`function(){%s}`, editEvent))
			}

		}
		deviceToggler = web.Scope(
			VBtnToggle(
				VBtn("").Icon("mdi-laptop").Color(ColorPrimary).Variant(VariantText).Class("mr-4").
					Attr("@click", web.Plaid().EventFunc(ReloadRenderPageOrTemplateEvent).Queries(ctx.R.Form).Query(paramsDevice, DeviceComputer).Go()),
				VBtn("").Icon("mdi-tablet").Color(ColorPrimary).Variant(VariantText).Class("mr-4").
					Attr("@click", web.Plaid().EventFunc(ReloadRenderPageOrTemplateEvent).Queries(ctx.R.Form).Query(paramsDevice, DeviceTablet).Go()),
				VBtn("").Icon("mdi-cellphone").Color(ColorPrimary).Variant(VariantText).Class("mr-4").
					Attr("@click", web.Plaid().EventFunc(ReloadRenderPageOrTemplateEvent).Queries(ctx.R.Form).Query(paramsDevice, DevicePhone).Go()),
			).Class("pa-2 rounded-lg ").Attr("v-model", "toggleLocals.activeDevice").Density(DensityCompact),
		).VSlot("{ locals : toggleLocals}").Init(fmt.Sprintf(`{activeDevice: %d}`, activeDevice))
		if tabContent, err = m.pageContent(ctx, obj); err != nil {
			return
		}
		if p, ok := obj.(publish.StatusInterface); ok {
			ctx.R.Form.Set(paramStatus, p.EmbedStatus().Status)
		}
		versionComponent = publish.DefaultVersionComponentFunc(m.mb, publish.VersionComponentConfig{Top: true})(obj, &presets.FieldContext{ModelInfo: m.mb.Info()}, ctx)
		exitHref = m.mb.Info().DetailingHref(ctx.Param(presets.ParamID))
		pageAppbarContent = h.Components(
			h.Div(
				h.Div(VIcon("mdi-exit-to-app").
					Attr("@click", web.Plaid().URL(exitHref).PushState(true).Go())).Style("transform:rotateY(180deg)").Class("mr-4"),
				VAppBarTitle().Text("Page Builder"),
			).Class("d-inline-flex align-center"),
			h.Div(deviceToggler).Class("text-center  w-25 d-flex justify-space-between ml-8"),
			versionComponent,
		)
		if navigatorDrawer, err = b.renderNavigator(ctx, m); err != nil {
			return
		}
		r.Body = h.Components(
			VAppBar(
				h.Div(
					pageAppbarContent...,
				).Class("d-flex align-center  justify-space-between  w-100 px-6").Style("height: 36px"),
			).Elevation(0).Density(DensityCompact).Height(96).Class("align-center border-b"),
			VNavigationDrawer(
				navigatorDrawer,
			).Location(LocationLeft).
				Permanent(true).
				Width(350),
			VNavigationDrawer(
				web.Portal(editContainerDrawer).Name(pageBuilderRightContentPortal),
			).Location(LocationRight).
				Permanent(true).
				Width(350),
			VMain(
				vx.VXMessageListener().ListenFunc(b.generateEditorBarJsFunction(ctx)),
				tabContent.Body.(h.HTMLComponent),
			),
		)
		return
	}
}

func (b *Builder) getDevice(ctx *web.EventContext) (device string, style string) {
	device = ctx.R.FormValue(paramsDevice)
	if len(device) == 0 {
		device = b.defaultDevice
	}

	switch device {
	case DevicePhone:
		style = "414px"
	case DeviceTablet:
		style = "768px"
		// case Device_Computer:
		//	style = "width: 1264px;"
	}

	return
}

const ContainerToPageLayoutKey = "ContainerToPageLayout"

type ContainerSorterItem struct {
	Index          int    `json:"index"`
	Label          string `json:"label"`
	ModelName      string `json:"model_name"`
	ModelID        string `json:"model_id"`
	DisplayName    string `json:"display_name"`
	ContainerID    string `json:"container_id"`
	URL            string `json:"url"`
	Shared         bool   `json:"shared"`
	Hidden         bool   `json:"hidden"`
	VisibilityIcon string `json:"visibility_icon"`
	ParamID        string `json:"param_id"`
	Locale         string `json:"locale"`
}

type ContainerSorter struct {
	Items []ContainerSorterItem `json:"items"`
}

func (b *Builder) renderNavigator(ctx *web.EventContext, m *ModelBuilder) (r h.HTMLComponent, err error) {
	tab := ctx.Param(paramTab)
	if tab == "" {
		tab = EditorTabElements
	}

	var listContainers h.HTMLComponent
	if tab == EditorTabLayers {
		if listContainers, err = m.renderContainersSortedList(ctx); err != nil {
			return
		}
	}
	r = h.Components(
		VTabs(
			VTab().Text("Add").
				Value(EditorTabElements).Attr("@click",
				web.Plaid().PushState(true).MergeQuery(true).
					Query(paramTab, EditorTabElements).RunPushState()),
			VTab().Text("Layers").Value(EditorTabLayers).Attr("@click",
				web.Plaid().PushState(true).MergeQuery(true).
					Query(paramTab, EditorTabLayers).RunPushState()+
					";"+
					web.Plaid().
						URL(ctx.R.URL.Path).
						EventFunc(ShowSortedContainerDrawerEvent).
						Query(paramStatus, ctx.Param(paramStatus)).
						MergeQuery(true).
						Go()),
		).Attr("v-model", "vars.containerTab").FixedTabs(true),
		VTabsWindow(
			VTabsWindowItem(m.renderContainersList(ctx)).Value(EditorTabElements),
			VTabsWindowItem(web.Portal(listContainers).Name(pageBuilderLayerContainerPortal)).Value(EditorTabLayers),
		).Attr("v-model", "vars.containerTab").Attr(web.VAssign("vars", fmt.Sprintf(`{containerTab:"%s"}`, tab))...),
	)
	return
}

func (b *Builder) renderEditContainer(ctx *web.EventContext) (r h.HTMLComponent, err error) {
	var (
		modelName     = ctx.R.FormValue(paramModelName)
		containerName = ctx.R.FormValue(paramContainerName)
		modelID       = ctx.R.FormValue(paramModelID)
	)
	builder := b.ContainerByName(modelName).GetModelBuilder()
	element := builder.NewModel()
	if err = b.db.First(element, modelID).Error; err != nil {
		return
	}
	r = web.Scope(
		VLayout(
			VMain(
				h.Div(
					h.Span(containerName).Class("text-subtitle-1"),
					h.Div(
						VBtn("Save").Variant(VariantFlat).Color(ColorSecondary).Size(SizeSmall).Attr("@click", web.Plaid().
							EventFunc(actions.Update).
							URL(b.ContainerByName(modelName).mb.Info().ListingHref()).
							Query(presets.ParamID, modelID).
							Go()),
					),
				).Class("d-flex  pa-6 align-center justify-space-between"),
				VDivider(),
				h.Div(

					builder.Editing().ToComponent(builder.Info(), element, ctx),
				).Class("pa-6"),
			),
		),
	).VSlot("{ form }")
	return
}

func (b *Builder) copyContainersToNewPageVersion(db *gorm.DB, pageID int, locale, oldPageVersion, newPageVersion string) (err error) {
	return b.copyContainersToAnotherPage(db, pageID, oldPageVersion, locale, pageID, newPageVersion, locale)
}

func (b *Builder) copyContainersToAnotherPage(db *gorm.DB, pageID int, pageVersion, locale string, toPageID int, toPageVersion, toPageLocale string) (err error) {
	var cons []*Container
	err = db.Order("display_order ASC").Find(&cons, "page_id = ? AND page_version = ? AND locale_code = ?", pageID, pageVersion, locale).Error
	if err != nil {
		return
	}

	for _, c := range cons {
		newModelID := c.ModelID
		if !c.Shared {
			model := b.ContainerByName(c.ModelName).NewModel()
			if err = db.First(model, "id = ?", c.ModelID).Error; err != nil {
				return
			}
			if err = reflectutils.Set(model, "ID", uint(0)); err != nil {
				return
			}
			if err = db.Create(model).Error; err != nil {
				return
			}
			newModelID = reflectutils.MustGet(model, "ID").(uint)
		}

		if err = db.Create(&Container{
			PageID:       uint(toPageID),
			PageVersion:  toPageVersion,
			ModelName:    c.ModelName,
			DisplayName:  c.DisplayName,
			ModelID:      newModelID,
			DisplayOrder: c.DisplayOrder,
			Shared:       c.Shared,
			Locale: l10n.Locale{
				LocaleCode: toPageLocale,
			},
		}).Error; err != nil {
			return
		}
	}
	return
}

func (b *Builder) localizeContainersToAnotherPage(db *gorm.DB, pageID int, pageVersion, locale string, toPageID int, toPageVersion, toPageLocale string) (err error) {
	var cons []*Container
	err = db.Order("display_order ASC").Find(&cons, "page_id = ? AND page_version = ? AND locale_code = ?", pageID, pageVersion, locale).Error
	if err != nil {
		return
	}

	for _, c := range cons {
		newModelID := c.ModelID
		newDisplayName := c.DisplayName
		if !c.Shared {
			model := b.ContainerByName(c.ModelName).NewModel()
			if err = db.First(model, "id = ?", c.ModelID).Error; err != nil {
				return
			}
			if err = reflectutils.Set(model, "ID", uint(0)); err != nil {
				return
			}
			if err = db.Create(model).Error; err != nil {
				return
			}
			newModelID = reflectutils.MustGet(model, "ID").(uint)
		} else {
			var count int64
			var sharedCon Container
			if err = db.Where("model_name = ? AND localize_from_model_id = ? AND locale_code = ? AND shared = ?", c.ModelName, c.ModelID, toPageLocale, true).First(&sharedCon).Count(&count).Error; err != nil && err != gorm.ErrRecordNotFound {
				return
			}

			if count == 0 {
				model := b.ContainerByName(c.ModelName).NewModel()
				if err = db.First(model, "id = ?", c.ModelID).Error; err != nil {
					return
				}
				if err = reflectutils.Set(model, "ID", uint(0)); err != nil {
					return
				}
				if err = db.Create(model).Error; err != nil {
					return
				}
				newModelID = reflectutils.MustGet(model, "ID").(uint)
			} else {
				newModelID = sharedCon.ModelID
				newDisplayName = sharedCon.DisplayName
			}
		}

		var newCon Container
		err = db.Order("display_order ASC").Find(&newCon, "id = ? AND locale_code = ?", c.ID, toPageLocale).Error
		if err != nil {
			return
		}

		newCon.ID = c.ID
		newCon.PageID = uint(toPageID)
		newCon.PageVersion = toPageVersion
		newCon.ModelName = c.ModelName
		newCon.DisplayName = newDisplayName
		newCon.ModelID = newModelID
		newCon.DisplayOrder = c.DisplayOrder
		newCon.Shared = c.Shared
		newCon.LocaleCode = toPageLocale
		newCon.LocalizeFromModelID = c.ModelID

		if err = db.Save(&newCon).Error; err != nil {
			return
		}
	}
	return
}

func (b *Builder) localizeCategory(db *gorm.DB, fromCategoryID uint, fromLocale string, toLocale string) (err error) {
	if fromCategoryID == 0 {
		return
	}
	var category Category
	var toCategory Category
	err = db.First(&category, "id = ? AND locale_code = ?", fromCategoryID, fromLocale).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
		return
	}
	if err != nil {
		return
	}
	err = db.First(&toCategory, "id = ? AND locale_code = ?", fromCategoryID, toLocale).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		category.LocaleCode = toLocale
		err = db.Save(&category).Error
		return
	}
	return
}

func (b *Builder) createModelAfterLocalizeDemoContainer(db *gorm.DB, c *DemoContainer) (err error) {
	model := b.ContainerByName(c.ModelName).NewModel()
	if err = db.First(model, "id = ?", c.ModelID).Error; err != nil {
		return
	}
	if err = reflectutils.Set(model, "ID", uint(0)); err != nil {
		return
	}
	if err = db.Create(model).Error; err != nil {
		return
	}

	c.ModelID = reflectutils.MustGet(model, "ID").(uint)
	return
}

func (b *Builder) markAsSharedContainer(ctx *web.EventContext) (r web.EventResponse, err error) {
	var container Container
	paramID := ctx.R.FormValue(paramContainerID)
	cs := container.PrimaryColumnValuesBySlug(paramID)
	containerID := cs["id"]
	locale := cs["locale_code"]

	err = b.db.Model(&Container{}).Where("id = ? AND locale_code = ?", containerID, locale).Update("shared", true).Error
	if err != nil {
		return
	}
	r.PushState = web.Location(url.Values{})
	return
}

type editorContainer struct {
	builder   *ContainerBuilder
	container *Container
}

func (b *Builder) getContainerBuilders(cs []*Container) (r []*editorContainer) {
	for _, c := range cs {
		for _, cb := range b.containerBuilders {
			if cb.name == c.ModelName {
				r = append(r, &editorContainer{
					builder:   cb,
					container: c,
				})
			}
		}
	}
	return
}

const (
	dialogPortalName = "pagebuilder_DialogPortalName"
)

func (b *Builder) pageEditorLayout(in web.PageFunc, config *presets.LayoutConfig) (out web.PageFunc) {
	return func(ctx *web.EventContext) (pr web.PageResponse, err error) {
		b.ps.InjectAssets(ctx)
		var innerPr web.PageResponse
		innerPr, err = in(ctx)
		if err != nil {
			panic(err)
		}
		pr.PageTitle = fmt.Sprintf("%s - %s", innerPr.PageTitle, "Page Builder")
		pr.Body = VLayout(

			web.Portal().Name(presets.RightDrawerPortalName),
			web.Portal().Name(presets.DialogPortalName),
			web.Portal().Name(presets.DeleteConfirmPortalName),
			web.Portal().Name(presets.ListingDialogPortalName),
			web.Portal().Name(dialogPortalName),
			innerPr.Body.(h.HTMLComponent),
		).Attr("id", "vt-app").
			Attr(web.VAssign("vars", `{presetsRightDrawer: false, presetsDialog: false, dialogPortalName: false}`)...)
		return
	}
}

func scrollToContainer(containerDataID interface{}) string {
	return fmt.Sprintf(`vars.el.refs.scrollIframe.scrollToCurrentContainer(%v);`, containerDataID)
}

func (b *Builder) containerWrapper(r *h.HTMLTagBuilder, ctx *web.EventContext, isEditor, isReadonly, isFirst, isEnd bool, containerDataID, modelName string, input *RenderInput) h.HTMLComponent {
	r.Attr("data-container-id", containerDataID)
	pmb := postMessageBody{
		ContainerDataID: containerDataID,
		ContainerId:     input.ContainerId,
		DisplayName:     input.DisplayName,
		ModelName:       modelName,
		isFirst:         isFirst,
		isEnd:           isEnd,
	}
	if isEditor {
		if isReadonly {
			r.AppendChildren(h.Div().Class("wrapper-shadow"))
		} else {
			r.AppendChildren(h.Div().Class("inner-shadow"))
			r = h.Div(
				r.Attr("onclick", "event.stopPropagation();document.querySelectorAll('.highlight').forEach(item=>{item.classList.remove('highlight')});this.parentElement.classList.add('highlight');"+pmb.postMessage(EventEdit)),
				h.Div(
					h.H6(input.DisplayName).Class("title"),
					h.Div(
						h.Button("").Children(h.I("arrow_upward").Class("material-icons")).Attr("onclick", pmb.postMessage(EventUp)),
						h.Button("").Children(h.I("arrow_downward").Class("material-icons")).Attr("onclick", pmb.postMessage(EventDown)),
						h.Button("").Children(h.I("delete").Class("material-icons")).Attr("onclick", pmb.postMessage(EventDelete)),
					).Class("editor-bar-buttons"),
				).Class("editor-bar"),
				h.Div(
					h.Div().Class("add"),
					h.Button("").Children(h.I("add").Class("material-icons")).Attr("onclick", pmb.postMessage(EventAdd)),
				).Class("editor-add"),
			).Class("wrapper-shadow").ClassIf("highlight", ctx.Param(paramContainerDataID) == containerDataID)
		}
	}
	return r
}

type (
	postMessageBody struct {
		MsgType         string `json:"msg_type"`
		ContainerDataID string `json:"container_data_id"`
		ContainerId     string `json:"container_id"`
		DisplayName     string `json:"display_name"`
		ModelName       string `json:"model_name"`
		isFirst         bool
		isEnd           bool
	}
)

func (b *postMessageBody) postMessage(msgType string) string {
	if msgType == EventUp && b.isFirst {
		return ""
	}
	if msgType == EventDown && b.isEnd {
		return ""
	}
	b.MsgType = msgType
	return fmt.Sprintf(`window.parent.postMessage(%s, '*')`, h.JSONString(b))
}
