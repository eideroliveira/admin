package views

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/goplaid/web"
	"github.com/goplaid/x/i18n"
	"github.com/goplaid/x/presets"
	. "github.com/goplaid/x/vuetify"
	"github.com/jinzhu/gorm"
	"github.com/qor/qor5/fileicons"
	"github.com/qor/qor5/media"
	"github.com/qor/qor5/media/media_library"
	"github.com/sunfmin/reflectutils"
	h "github.com/theplant/htmlgo"
	"golang.org/x/text/language"
)

type MediaBoxConfigKey int

var MediaLibraryPerPage int64 = 39

const MediaBoxConfig MediaBoxConfigKey = iota
const I18nMediaLibraryKey i18n.ModuleKey = "I18nMediaLibraryKey"

func Configure(b *presets.Builder, db *gorm.DB) {
	b.FieldDefaults(presets.WRITE).
		FieldType(media_library.MediaBox{}).
		ComponentFunc(MediaBoxComponentFunc(db)).
		SetterFunc(MediaBoxSetterFunc(db))

	b.FieldDefaults(presets.LIST).
		FieldType(media_library.MediaBox{}).
		ComponentFunc(MediaBoxListFunc())

	registerEventFuncs(b.GetWebBuilder(), db)

	b.I18n().
		RegisterForModule(language.English, I18nMediaLibraryKey, Messages_en_US).
		RegisterForModule(language.SimplifiedChinese, I18nMediaLibraryKey, Messages_zh_CN)
}

func MediaBoxComponentFunc(db *gorm.DB) presets.FieldComponentFunc {
	return func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		cfg := field.ContextValue(MediaBoxConfig).(*media_library.MediaBoxConfig)
		mediaBox := field.Value(obj).(media_library.MediaBox)
		return QMediaBox(db).
			FieldName(field.Name).
			Value(&mediaBox).
			Label(field.Label).
			Config(cfg)
	}
}

func MediaBoxSetterFunc(db *gorm.DB) presets.FieldSetterFunc {
	return func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) (err error) {
		jsonValuesField := fmt.Sprintf("%s.Values", field.Name)
		mediaBox := media_library.MediaBox{}
		err = mediaBox.Scan(ctx.R.FormValue(jsonValuesField))
		if err != nil {
			return
		}
		descriptionField := fmt.Sprintf("%s.Description", field.Name)
		mediaBox.Description = ctx.R.FormValue(descriptionField)
		err = reflectutils.Set(obj, field.Name, mediaBox)
		if err != nil {
			return
		}

		return
	}
}

type QMediaBoxBuilder struct {
	fieldName string
	label     string
	value     *media_library.MediaBox
	config    *media_library.MediaBoxConfig
	db        *gorm.DB
}

func QMediaBox(db *gorm.DB) (r *QMediaBoxBuilder) {
	r = &QMediaBoxBuilder{
		db: db,
	}
	return
}

func (b *QMediaBoxBuilder) FieldName(v string) (r *QMediaBoxBuilder) {
	b.fieldName = v
	return b
}

func (b *QMediaBoxBuilder) Value(v *media_library.MediaBox) (r *QMediaBoxBuilder) {
	b.value = v
	return b
}

func (b *QMediaBoxBuilder) Label(v string) (r *QMediaBoxBuilder) {
	b.label = v
	return b
}

func (b *QMediaBoxBuilder) Config(v *media_library.MediaBoxConfig) (r *QMediaBoxBuilder) {
	b.config = v
	return b
}

func (b *QMediaBoxBuilder) MarshalHTML(c context.Context) (r []byte, err error) {
	if len(b.fieldName) == 0 {
		panic("FieldName required")
	}
	if b.value == nil {
		panic("Value required")
	}

	ctx := web.MustGetEventContext(c)
	registerEventFuncs(ctx.Hub, b.db)

	portalName := mainPortalName(b.fieldName)

	return h.Components(
		VSheet(
			h.If(len(b.label) > 0,
				h.Label(b.label).Class("v-label theme--light"),
			),
			web.Portal(
				mediaBoxThumbnails(ctx, b.value, b.fieldName, b.config),
			).Name(mediaBoxThumbnailsPortalName(b.fieldName)),
			web.Portal().Name(portalName),
		).Class("pb-4").
			Rounded(true).
			Attr(web.InitContextVars, `{showFileChooser: false}`),
	).MarshalHTML(c)
}

func mediaBoxThumb(msgr *Messages, cfg *media_library.MediaBoxConfig,
	f *media_library.MediaBox, field string, thumb string) h.HTMLComponent {
	size := cfg.Sizes[thumb]
	fileSize := f.FileSizes[thumb]
	url := f.URL(thumb)
	if thumb == media.DefaultSizeKey {
		url = f.URL()
	}
	return VCard(
		h.If(media.IsImageFormat(f.FileName),
			VImg().Src(fmt.Sprintf("%s?%d", url, time.Now().UnixNano())).Height(150),
		).Else(
			h.Div(
				fileThumb(f.FileName),
				h.A().Text(f.FileName).Href(f.Url).Target("_blank"),
			).Style("text-align:center"),
		),
		h.If(media.IsImageFormat(f.FileName) && (size != nil || thumb == media.DefaultSizeKey),
			VCardActions(
				VChip(
					thumbName(thumb, size, fileSize, f),
				).Small(true).Attr("@click", web.Plaid().
					EventFunc(loadImageCropperEvent, field, fmt.Sprint(f.ID), thumb, h.JSONString(cfg)).
					Go()),
			),
		),
	)
}

func fileThumb(filename string) h.HTMLComponent {
	return h.Div(
		fileicons.Icon(path.Ext(filename)[1:]).Attr("height", "150").Class("pt-4"),
	).Class("d-flex align-center justify-center")
}

func deleteConfirmation(db *gorm.DB) web.EventFunc {
	return func(ctx *web.EventContext) (r web.EventResponse, err error) {
		msgr := i18n.MustGetModuleMessages(ctx.R, presets.CoreI18nModuleKey, Messages_en_US).(*presets.Messages)
		field := ctx.Event.Params[0]
		id := ctx.Event.Params[1]
		cfg := ctx.Event.Params[2]

		r.UpdatePortals = append(r.UpdatePortals, &web.PortalUpdate{
			Name: deleteConfirmPortalName(field),
			Body: VDialog(
				VCard(
					VCardTitle(h.Text(msgr.DeleteConfirmationText(id))),
					VCardActions(
						VSpacer(),
						VBtn(msgr.Cancel).
							Depressed(true).
							Class("ml-2").
							On("click", "vars.mediaLibrary_deleteConfirmation = false"),

						VBtn(msgr.Delete).
							Color("primary").
							Depressed(true).
							Dark(true).
							Attr("@click", web.Plaid().
								EventFunc(doDeleteEvent, field, id, h.JSONString(stringToCfg(cfg))).
								Go()),
					),
				),
			).MaxWidth("600px").
				Attr("v-model", "vars.mediaLibrary_deleteConfirmation").
				Attr(web.InitContextVars, `{mediaLibrary_deleteConfirmation: false}`),
		})

		r.VarsScript = "setTimeout(function(){ vars.mediaLibrary_deleteConfirmation = true }, 100)"
		return
	}
}
func doDelete(db *gorm.DB) web.EventFunc {
	return func(ctx *web.EventContext) (r web.EventResponse, err error) {
		field := ctx.Event.Params[0]
		id := ctx.Event.Params[1]
		cfg := ctx.Event.Params[2]

		err = db.Delete(&media_library.MediaLibrary{}, "id = ?", id).Error
		if err != nil {
			panic(err)
		}

		renderFileChooserDialogContent(
			ctx,
			&r,
			field,
			db,
			stringToCfg(cfg),
		)
		r.VarsScript = "vars.mediaLibrary_deleteConfirmation = false"
		return
	}
}

func mediaBoxThumbnails(ctx *web.EventContext, mediaBox *media_library.MediaBox, field string, cfg *media_library.MediaBoxConfig) h.HTMLComponent {
	msgr := i18n.MustGetModuleMessages(ctx.R, I18nMediaLibraryKey, Messages_en_US).(*Messages)
	c := VContainer().Fluid(true)

	if mediaBox.ID.String() != "" {
		row := VRow()
		if len(cfg.Sizes) == 0 {
			row.AppendChildren(
				VCol(
					mediaBoxThumb(msgr, cfg, mediaBox, field, media.DefaultSizeKey),
				).Cols(6).Sm(4).Class("pl-0"),
			)
		} else {
			var keys []string
			for k, _ := range cfg.Sizes {
				keys = append(keys, k)
			}

			sort.Strings(keys)

			for _, k := range keys {
				row.AppendChildren(
					VCol(
						mediaBoxThumb(msgr, cfg, mediaBox, field, k),
					).Cols(6).Sm(4).Class("pl-0"),
				)
			}
		}

		c.AppendChildren(row)

		if media.IsImageFormat(mediaBox.FileName) {
			fieldName := fmt.Sprintf("%s.Description", field)
			value := ctx.R.FormValue(fieldName)
			if len(value) == 0 {
				value = mediaBox.Description
			}
			c.AppendChildren(
				VRow(
					VCol(
						VTextField().
							Value(value).
							Attr(web.VFieldName(fieldName)...).
							Label(msgr.DescriptionForAccessibility).
							Dense(true).
							HideDetails(true).
							Outlined(true),
					).Cols(12).Class("pl-0 pt-0"),
				),
			)
		}
	}

	mediaBoxValue := ""
	if mediaBox.ID.String() != "" {
		mediaBoxValue = h.JSONString(mediaBox)
	}

	return h.Components(
		c,
		web.Portal().Name(cropperPortalName(field)),
		h.Input("").Type("hidden").
			Value(mediaBoxValue).
			Attr(web.VFieldName(fmt.Sprintf("%s.Values", field))...),
		VBtn(msgr.ChooseFile).
			Depressed(true).
			OnClick(openFileChooserEvent, field, h.JSONString(cfg)),
		h.If(mediaBox != nil && mediaBox.ID.String() != "",
			VBtn(msgr.Delete).
				Depressed(true).
				OnClick(deleteFileEvent, field, h.JSONString(cfg)),
		),
	)
}

func MediaBoxListFunc() presets.FieldComponentFunc {
	return func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		mediaBox := field.Value(obj).(media_library.MediaBox)
		return h.Td(h.Img("").Src(mediaBox.URL("@qor_preview")).Style("height: 48px;"))
	}
}

func deleteFileField() web.EventFunc {
	return func(ctx *web.EventContext) (r web.EventResponse, err error) {
		field := ctx.Event.Params[0]
		cfg := stringToCfg(ctx.Event.Params[1])
		r.UpdatePortals = append(r.UpdatePortals, &web.PortalUpdate{
			Name: mediaBoxThumbnailsPortalName(field),
			Body: mediaBoxThumbnails(ctx, &media_library.MediaBox{}, field, cfg),
		})
		return
	}
}

func stringToCfg(v string) *media_library.MediaBoxConfig {
	var cfg media_library.MediaBoxConfig
	if len(v) == 0 {
		return &cfg
	}
	err := json.Unmarshal([]byte(v), &cfg)
	if err != nil {
		panic(err)
	}

	return &cfg
}

func thumbName(name string, size *media.Size, fileSize int, f *media_library.MediaBox) h.HTMLComponent {
	text := name
	if size != nil {
		text = fmt.Sprintf("%s(%dx%d)", text, size.Width, size.Height)
	}
	if name == media.DefaultSizeKey {
		text = fmt.Sprintf("%s(%dx%d)", text, f.Width, f.Height)
	}
	if fileSize != 0 {
		text = fmt.Sprintf("%s %s", text, media.ByteCountSI(fileSize))
	}
	return h.Text(text)
}

func updateDescription(db *gorm.DB) web.EventFunc {
	return func(ctx *web.EventContext) (r web.EventResponse, err error) {
		//field := ctx.Event.Params[0]
		id := ctx.Event.ParamAsInt(1)

		var media media_library.MediaLibrary
		if err = db.Find(&media, id).Error; err != nil {
			return
		}

		media.File.Description = ctx.R.FormValue("CurrentDescription")
		if err = db.Save(&media).Error; err != nil {
			return
		}

		r.VarsScript = `vars.snackbarShow = true;`
		return
	}
}
