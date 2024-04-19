package containers

import (
	"fmt"

	"github.com/iancoleman/strcase"
	"github.com/jinzhu/inflection"

	"github.com/qor5/admin/v3/pagebuilder"
	"github.com/qor5/admin/v3/presets"
	"github.com/qor5/admin/v3/richeditor"
	"github.com/qor5/ui/v3/vuetify"
	"github.com/qor5/web/v3"
	. "github.com/theplant/htmlgo"
	"gorm.io/gorm"
)

const (
	LINK_DISPLAY_OPTION_DESKTOP = "desktop"
	LINK_DISPLAY_OPTION_MOBILE  = "mobile"
	LINK_DISPLAY_OPTION_ALL     = "all"
)

var LinkDisplayOptions = []string{LINK_DISPLAY_OPTION_ALL, LINK_DISPLAY_OPTION_DESKTOP, LINK_DISPLAY_OPTION_MOBILE}

type Heading struct {
	ID                uint
	AddTopSpace       bool
	AddBottomSpace    bool
	AnchorID          string
	Heading           string
	FontColor         string
	BackgroundColor   string
	Link              string
	LinkText          string
	LinkDisplayOption string
	Text              string
}

func (*Heading) TableName() string {
	return "container_headings"
}

func RegisterHeadingContainer(pb *pagebuilder.Builder, db *gorm.DB) {
	vb := pb.RegisterContainer("Heading", "Navigation").
		RenderFunc(func(obj interface{}, input *pagebuilder.RenderInput, ctx *web.EventContext) HTMLComponent {
			v := obj.(*Heading)
			return HeadingBody(v, input)
		})
	ed := vb.Model(&Heading{}).Editing("AddTopSpace", "AddBottomSpace", "AnchorID", "Heading", "FontColor", "BackgroundColor", "Link", "LinkText", "LinkDisplayOption", "Text")
	ed.Field("Text").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) HTMLComponent {
		return richeditor.RichEditor(db, "Text").Plugins([]string{"alignment", "video", "imageinsert", "fontcolor"}).Value(obj.(*Heading).Text).Label(field.Label)
	})

	ed.Field("FontColor").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) HTMLComponent {
		return vuetify.VSelect().
			Items(FontColors).
			Label(field.Label).
			Variant(vuetify.FieldVariantUnderlined).
			Attr(web.VField(field.FormKey, field.Value(obj))...)
	})
	ed.Field("BackgroundColor").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) HTMLComponent {
		return vuetify.VSelect().
			Items(BackgroundColors).
			Label(field.Label).
			Variant(vuetify.FieldVariantUnderlined).
			Attr(web.VField(field.FormKey, field.Value(obj))...)
	})
	ed.Field("LinkDisplayOption").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) HTMLComponent {
		return vuetify.VSelect().
			Items(LinkDisplayOptions).
			Label(field.Label).
			Variant(vuetify.FieldVariantUnderlined).
			Attr(web.VField(field.FormKey, field.Value(obj))...)
	})
}

func HeadingBody(data *Heading, input *pagebuilder.RenderInput) (body HTMLComponent) {
	headingBody :=
		Div(
			Div(
				If(data.Heading != "",
					If(data.Link != "",
						A(H2(data.Heading).Class("container-heading-title")).Class("container-heading-title-link").Href(data.Link),
					),
					If(data.Link == "",
						H2(data.Heading).Class("container-heading-title"),
					),
				),
				If(data.Text != "", Div(RawHTML(data.Text)).Class("container-heading-content")),
			).Class("container-heading-wrap"),
			If(data.LinkText != "" && data.Link != "",
				Div(
					LinkTextWithArrow(data.LinkText, data.Link),
				).Class("container-heading-link").Attr("data-display", data.LinkDisplayOption),
			),
		).Class("container-heading-inner")

	body = ContainerWrapper(
		fmt.Sprintf(inflection.Plural(strcase.ToKebab("Heading"))+"_%v", data.ID), data.AnchorID, "container-heading", data.BackgroundColor, "", data.FontColor,
		"", data.AddTopSpace, data.AddBottomSpace, input.IsEditor, input.IsReadonly, "", input,
		Div(headingBody).Class("container-wrapper"),
	)
	return
}
