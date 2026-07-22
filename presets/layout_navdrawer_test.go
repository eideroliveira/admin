package presets

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	h "github.com/theplant/htmlgo"

	"github.com/stretchr/testify/require"
)

// The layout root used to read localStorage through a `vars.__window`
// binding that web/corejs deliberately no longer exposes — Vue's
// template sandbox rewrites bare browser globals to _ctx.* (undefined),
// so the render function threw `Cannot read properties of undefined
// (reading 'localStorage')` and every admin page (the page builder
// editor included) died before mounting.
//
// Persistence now lives in a v-on-mounted lifecycle callback that
// receives the real window. Guard both halves: no escape hatch in the
// rendered markup, and the callback still wired up.
func TestDefaultLayoutCompoDoesNotReachWindowThroughVars(t *testing.T) {
	b := New()
	compo := b.defaultLayoutCompo(nil, h.Div(), h.Div())

	var buf bytes.Buffer
	require.NoError(t, h.Fprint(&buf, compo, context.Background()))
	got := buf.String()

	require.NotContains(t, got, "__window",
		"layout root must not reach browser globals through vars")
	require.Contains(t, got, "v-on-mounted",
		"nav drawer persistence must run in a lifecycle callback")
	require.Contains(t, got, "gordpress.navDrawer",
		"nav drawer state should still be persisted")
}

// The persist callback is a JS literal embedded in a template, so an
// accidental reference to a sandboxed global only surfaces at runtime in
// the browser. Every storage access must go through the `window` the
// lifecycle directive injects — a bare `localStorage` would compile to
// _ctx.localStorage and throw exactly the way vars.__window did.
func TestNavDrawerPersistScriptUsesInjectedWindow(t *testing.T) {
	require.True(t, strings.HasPrefix(navDrawerPersistScript, "({window, watch})"),
		"callback must destructure the window/watch the directive injects")
	require.NotContains(t, navDrawerPersistScript, "vars.__window")
	for _, ref := range regexp.MustCompile(`\b\w*\.?localStorage\b`).
		FindAllString(navDrawerPersistScript, -1) {
		require.Equal(t, "window.localStorage", ref,
			"storage must be reached through the injected window, not a bare global")
	}
}
