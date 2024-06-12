package examples_web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/qor5/admin/v3/docs/docsrc/examples"
	"github.com/qor5/web/v3"
	. "github.com/theplant/htmlgo"
)

type PortalCompo interface {
	HTMLComponent
	PortalName() string
}

func Portalize[T PortalCompo](compo T) HTMLComponent {
	return web.Portal(compo).Name(compo.PortalName())
}

func ReloadCompo[T PortalCompo](c T, f func(cloned T)) string {
	cloned := MustClone(c)
	f(cloned)
	return CompoAction(cloned, compoActionReload, struct{}{}).Go()
}

type SampleCompo struct {
	Id      string
	ModelId string
	ShowPre bool
}

func (c *SampleCompo) PortalName() string {
	return fmt.Sprintf("SampleCompo:%s", c.Id)
}

func (c *SampleCompo) MarshalHTML(ctx context.Context) ([]byte, error) {
	return Div(
		Iff(c.ShowPre, func() HTMLComponent {
			return Pre(JSONString(c))
		}),
		Button("SwitchShowPre").Attr("@click", ReloadCompo(c, func(cloned *SampleCompo) {
			cloned.ShowPre = !cloned.ShowPre
		})),
		Button("DeleteItem").Attr("@click", CompoAction(c, "DeleteItem", DeleteItemRequest{
			ModelId: c.ModelId,
		}).Go()),
	).MarshalHTML(ctx)
}

type DeleteItemRequest struct {
	ModelId string
}

func (c *SampleCompo) OnDeleteItem(req DeleteItemRequest) (r web.EventResponse, err error) {
	// TODO: 直接 alert 和 console.log 都不行
	r.RunScript = fmt.Sprintf("console.log('Delete item %s')", req.ModelId)
	return
}

func CompoExample(cx *web.EventContext) (pr web.PageResponse, err error) {
	pr.Body = Components(
		Portalize(&SampleCompo{Id: "666", ModelId: "model666"}),
		Br(),
		Portalize(&SampleCompo{Id: "888", ModelId: "model888"}),
	)
	return
}

func Copy(dst, src any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func Clone[T any](src T) (T, error) {
	var dst T
	if err := Copy(&dst, src); err != nil {
		return dst, err
	}
	return dst, nil
}

func MustClone[T any](src T) T {
	dst, err := Clone(src)
	if err != nil {
		panic(err)
	}
	return dst
}

// TODO: use a wrapper struct ?
const (
	paramCompoType          = "__compo_type__"
	paramCompo              = "__compo__"
	paramCompoAction        = "__compo_action__"
	paramCompoActionPayload = "__compo_action_payload__"
)

const (
	compoActionReload = "Reload"
)

func CompoAction(compo HTMLComponent, compoAction any, compoActionPayload any) *web.VueEventTagBuilder {
	var actionName string
	switch v := compoAction.(type) {
	case string:
		actionName = v
	default:
		// TODO: 测试了不行，传方法指针进来拿不到原方法名称，只有方法定义
		fullName := reflect.TypeOf(v).String()
		parts := strings.Split(fullName, ".")
		actionName = parts[len(parts)-1]
		actionName = strings.TrimPrefix(actionName, "On")
	}

	vs := url.Values{}
	vs.Set(paramCompo, JSONString(compo))
	vs.Set(paramCompoType, fmt.Sprintf("%T", compo))
	vs.Set(paramCompoAction, actionName)
	vs.Set(paramCompoActionPayload, JSONString(compoActionPayload))
	return web.Plaid().EventFunc(eventDispatchCompoAction).StringQuery(vs.Encode())
}

const eventDispatchCompoAction = "dispatchCompoAction"

func eventDispatchCompoActionHandler(ctx *web.EventContext) (r web.EventResponse, err error) {
	valCompo := ctx.R.FormValue(paramCompo)
	valCompoType := ctx.R.FormValue(paramCompoType)
	valCompoAction := ctx.R.FormValue(paramCompoAction)
	valCompoPayload := ctx.R.FormValue(paramCompoActionPayload)

	compo, err := newInstance(valCompoType)
	if err != nil {
		return r, err
	}

	err = json.Unmarshal([]byte(valCompo), compo)
	if err != nil {
		return r, err
	}

	// 通过反射找到并调用自定义 action 方法 OnXXX
	actionMethod := reflect.ValueOf(compo).MethodByName("On" + valCompoAction)
	if actionMethod.IsValid() && actionMethod.Kind() == reflect.Func {
		// 检查返回值类型是否符合 OnXXXX(payload PayloadXXXX) (r web.EventResponse, err error)
		actionMethodType := actionMethod.Type()
		if actionMethodType.NumOut() != 2 ||
			actionMethodType.Out(0) != reflect.TypeOf(web.EventResponse{}) ||
			actionMethodType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			return r, fmt.Errorf("method On%s has incorrect signature", valCompoAction)
		}

		// 检查参数数量并反序列化参数 // TODO: 应该要支持无参数的方法
		if actionMethodType.NumIn() != 1 {
			return r, fmt.Errorf("method On%s has incorrect number of arguments", valCompoAction)
		}

		argType := actionMethodType.In(0)
		argValue := reflect.New(argType).Interface()
		err := json.Unmarshal([]byte(valCompoPayload), &argValue)
		if err != nil {
			return r, fmt.Errorf("failed to unmarshal action payload to %T: %v", argValue, err)
		}

		// 调用方法并处理返回值
		result := actionMethod.Call([]reflect.Value{reflect.ValueOf(argValue).Elem()})
		if len(result) != 2 || result[0].CanInterface() || result[1].CanInterface() {
			return r, fmt.Errorf("abnormal action result %v", result)
		}
		r = result[0].Interface().(web.EventResponse)
		err = result[1].Interface().(error)
		return r, err
	}

	// TODO: 这样写貌似不够优雅
	switch valCompoAction {
	case compoActionReload:
		portalCompo, ok := compo.(PortalCompo)
		if !ok {
			return r, fmt.Errorf("%s cannot handle the Reload action", valCompoType)
		}
		r.UpdatePortals = append(r.UpdatePortals, &web.PortalUpdate{
			Name: portalCompo.PortalName(),
			Body: portalCompo,
		})
	}
	return
}

var CompoExamplePB = web.Page(CompoExample).
	EventFunc(eventDispatchCompoAction, eventDispatchCompoActionHandler)

var CompoExamplePath = examples.URLPathByFunc(CompoExample)

// TODO: should use a singleton to handle this
var typeRegistry = make(map[string]reflect.Type)

func registerType(v interface{}) {
	t := reflect.TypeOf(v)
	typeRegistry[t.String()] = t
}

func newInstance(typeName string) (interface{}, error) {
	if t, ok := typeRegistry[typeName]; ok {
		return reflect.New(t.Elem()).Interface(), nil
	}
	return nil, fmt.Errorf("type not found: %s", typeName)
}

func init() {
	registerType((*SampleCompo)(nil))
}
