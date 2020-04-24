// +build go1.7

package httptreemux

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type IContextGroup interface {
	GET(path string, handler http.HandlerFunc)
	POST(path string, handler http.HandlerFunc)
	PUT(path string, handler http.HandlerFunc)
	PATCH(path string, handler http.HandlerFunc)
	DELETE(path string, handler http.HandlerFunc)
	HEAD(path string, handler http.HandlerFunc)
	OPTIONS(path string, handler http.HandlerFunc)

	NewContextGroup(path string) *ContextGroup
	NewGroup(path string) *ContextGroup
}

func TestContextParams(t *testing.T) {
	m := &contextData{
		params: map[string]string{"id": "123"},
		route:  "",
	}

	ctx := context.WithValue(context.Background(), contextDataKey, m)

	params := ContextParams(ctx)
	if params == nil {
		t.Errorf("expected '%#v', but got '%#v'", m, params)
	}

	if v := params["id"]; v != "123" {
		t.Errorf("expected '%s', but got '%#v'", m.params["id"], params["id"])
	}
}

func TestContextRoute(t *testing.T) {
	tests := []struct{
		name,
		expectedRoute string
	} {
		{
			name: "basic",
			expectedRoute: "/base/path",
		},
		{
			name: "params",
			expectedRoute: "/base/path/:id/items/:itemid",
		},
		{
			name: "catch-all",
			expectedRoute: "/base/*path",
		},
		{
			name: "empty",
			expectedRoute: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cd := &contextData{}
			if len(test.expectedRoute) > 0 {
				cd.route = test.expectedRoute
			}
			ctx := context.WithValue(context.Background(), contextDataKey, cd)

			gotRoute := ContextRoute(ctx)

			if test.expectedRoute != gotRoute {
				t.Errorf("ContextRoute didn't return the desired route\nexpected %s\ngot: %s", test.expectedRoute, gotRoute)
			}
		})
	}
}

func TestContextData(t *testing.T) {
	p := &contextData{
		route:  "route/path",
		params: map[string]string{"id": "123"},
	}

	ctx := context.WithValue(context.Background(), contextDataKey, p)

	ctxData := ContextData(ctx)
	pathValue := ctxData.Route()
	if pathValue != p.route {
		t.Errorf("expected '%s', but got '%s'", p, pathValue)
	}

	params := ctxData.Params()
	if v := params["id"]; v != "123" {
		t.Errorf("expected '%s', but got '%#v'", p.params["id"], params["id"])
	}
}

func TestContextDataWithEmptyParams(t *testing.T) {
	p := &contextData{
		route:  "route/path",
		params: nil,
	}

	ctx := context.WithValue(context.Background(), contextDataKey, p)
	params := ContextData(ctx).Params()
	if params == nil {
		t.Errorf("ContextData.Params should never return nil")
	}
}

func TestContextGroupMethods(t *testing.T) {
	for _, scenario := range scenarios {
		t.Run(scenario.description, func(t *testing.T) {
			testContextGroupMethods(t, scenario.RequestCreator, true, false)
			testContextGroupMethods(t, scenario.RequestCreator, false, false)
			testContextGroupMethods(t, scenario.RequestCreator, true, true)
			testContextGroupMethods(t, scenario.RequestCreator, false, true)
		})
	}
}

func testContextGroupMethods(t *testing.T, reqGen RequestCreator, headCanUseGet bool, useContextRouter bool) {
	t.Run(fmt.Sprintf("headCanUseGet %v, useContextRouter %v", headCanUseGet, useContextRouter), func(t *testing.T) {
		var result string
		makeHandler := func(method, expectedRoutePath string, hasParam bool) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				result = method

				// Test Legacy Accessor
				var v string
				v, ok := ContextParams(r.Context())["param"]
				if hasParam && !ok {
					t.Error("missing key 'param' in context from ContextParams")
				}

				ctxData := ContextData(r.Context())
				v, ok = ctxData.Params()["param"]
				if hasParam && !ok {
					t.Error("missing key 'param' in context from ContextData")
				}

				routePath := ctxData.Route()
				if routePath != expectedRoutePath {
					t.Errorf("Expected context to have route path '%s', saw %s", expectedRoutePath, routePath)
				}

				if headCanUseGet && (method == "GET" || v == "HEAD") {
					return
				}
				if hasParam && v != method {
					t.Errorf("invalid key 'param' in context; expected '%s' but got '%s'", method, v)
				}
			}
		}

		var router http.Handler
		var rootGroup IContextGroup

		if useContextRouter {
			root := NewContextMux()
			root.HeadCanUseGet = headCanUseGet
			t.Log(root.TreeMux.HeadCanUseGet)
			router = root
			rootGroup = root
		} else {
			root := New()
			root.HeadCanUseGet = headCanUseGet
			router = root
			rootGroup = root.UsingContext()
		}

		cg := rootGroup.NewGroup("/base").NewGroup("/user")
		cg.GET("/:param", makeHandler("GET", cg.group.path+"/:param", true))
		cg.POST("/:param", makeHandler("POST", cg.group.path+"/:param", true))
		cg.PATCH("/PATCH", makeHandler("PATCH", cg.group.path+"/PATCH", false))
		cg.PUT("/:param", makeHandler("PUT", cg.group.path+"/:param", true))
		cg.Handler("DELETE", "/:param", http.HandlerFunc(makeHandler("DELETE", cg.group.path+"/:param", true)))

		testMethod := func(method, expect string) {
			result = ""
			w := httptest.NewRecorder()
			r, _ := reqGen(method, "/base/user/"+method, nil)
			router.ServeHTTP(w, r)
			if expect == "" && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Method %s not expected to match but saw code %d", method, w.Code)
			}

			if result != expect {
				t.Errorf("Method %s got result %s", method, result)
			}
		}

		testMethod("GET", "GET")
		testMethod("POST", "POST")
		testMethod("PATCH", "PATCH")
		testMethod("PUT", "PUT")
		testMethod("DELETE", "DELETE")

		if headCanUseGet {
			t.Log("Test implicit HEAD with HeadCanUseGet = true")
			testMethod("HEAD", "GET")
		} else {
			t.Log("Test implicit HEAD with HeadCanUseGet = false")
			testMethod("HEAD", "")
		}

		cg.HEAD("/:param", makeHandler("HEAD", cg.group.path+"/:param", true))
		testMethod("HEAD", "HEAD")
	})
}

func TestNewContextGroup(t *testing.T) {
	router := New()
	group := router.NewGroup("/api")

	group.GET("/v1", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Write([]byte(`200 OK GET /api/v1`))
	})

	group.UsingContext().GET("/v2", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`200 OK GET /api/v2`))
	})

	tests := []struct {
		uri, expected string
	}{
		{"/api/v1", "200 OK GET /api/v1"},
		{"/api/v2", "200 OK GET /api/v2"},
	}

	for _, tc := range tests {
		r, err := http.NewRequest("GET", tc.uri, nil)
		if err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s: expected %d, but got %d", tc.uri, http.StatusOK, w.Code)
		}
		if got := w.Body.String(); got != tc.expected {
			t.Errorf("GET %s : expected %q, but got %q", tc.uri, tc.expected, got)
		}

	}
}

type ContextGroupHandler struct{}

//	adhere to the http.Handler interface
func (f ContextGroupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Write([]byte(`200 OK GET /api/v1`))
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
}

func TestNewContextGroupHandler(t *testing.T) {
	router := New()
	group := router.NewGroup("/api")

	group.UsingContext().Handler("GET", "/v1", ContextGroupHandler{})

	tests := []struct {
		uri, expected string
	}{
		{"/api/v1", "200 OK GET /api/v1"},
	}

	for _, tc := range tests {
		r, err := http.NewRequest("GET", tc.uri, nil)
		if err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s: expected %d, but got %d", tc.uri, http.StatusOK, w.Code)
		}
		if got := w.Body.String(); got != tc.expected {
			t.Errorf("GET %s : expected %q, but got %q", tc.uri, tc.expected, got)
		}
	}
}

func TestDefaultContext(t *testing.T) {
	router := New()
	ctx := context.WithValue(context.Background(), "abc", "def")
	expectContext := false

	router.GET("/abc", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		contextValue := r.Context().Value("abc")
		if expectContext {
			x, ok := contextValue.(string)
			if !ok || x != "def" {
				t.Errorf("Unexpected context key value: %+v", contextValue)
			}
		} else {
			if contextValue != nil {
				t.Errorf("Expected blank context but key had value %+v", contextValue)
			}
		}
	})

	r, err := http.NewRequest("GET", "/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	t.Log("Testing without DefaultContext")
	router.ServeHTTP(w, r)

	router.DefaultContext = ctx
	expectContext = true
	w = httptest.NewRecorder()
	t.Log("Testing with DefaultContext")
	router.ServeHTTP(w, r)
}

func TestContextMuxSimple(t *testing.T) {
	router := NewContextMux()
	ctx := context.WithValue(context.Background(), "abc", "def")
	expectContext := false

	router.GET("/abc", func(w http.ResponseWriter, r *http.Request) {
		contextValue := r.Context().Value("abc")
		if expectContext {
			x, ok := contextValue.(string)
			if !ok || x != "def" {
				t.Errorf("Unexpected context key value: %+v", contextValue)
			}
		} else {
			if contextValue != nil {
				t.Errorf("Expected blank context but key had value %+v", contextValue)
			}
		}
	})

	r, err := http.NewRequest("GET", "/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	t.Log("Testing without DefaultContext")
	router.ServeHTTP(w, r)

	router.DefaultContext = ctx
	expectContext = true
	w = httptest.NewRecorder()
	t.Log("Testing with DefaultContext")
	router.ServeHTTP(w, r)
}

func TestAddDataToContext(t *testing.T) {
	expectedRoute := "/expected/route"
	expectedParams := map[string]string{
		"test": "expected",
	}

	ctx := AddRouteDataToContext(context.Background(), &contextData{
		route: expectedRoute,
		params: expectedParams,
	})

	if gotData, ok := ctx.Value(contextDataKey).(*contextData); ok && gotData != nil {
		if gotData.route != expectedRoute {
			t.Errorf("Did not retrieve the desired route. Expected: %s; Got: %s", expectedRoute, gotData.route)
		}
		if !reflect.DeepEqual(expectedParams, gotData.params) {
			t.Errorf("Did not retrieve the desired parameters. Expected: %#v; Got: %#v", expectedParams, gotData.params)
		}
	} else {
		t.Error("failed to retrieve context data")
	}
}

func TestAddParamsToContext(t *testing.T) {
	expectedParams := map[string]string{
		"test": "expected",
	}

	ctx := AddParamsToContext(context.Background(), expectedParams)

	if gotData, ok := ctx.Value(contextDataKey).(*contextData); ok && gotData != nil {
		if !reflect.DeepEqual(expectedParams, gotData.params) {
			t.Errorf("Did not retrieve the desired parameters. Expected: %#v; Got: %#v", expectedParams, gotData.params)
		}
	} else {
		t.Error("failed to retrieve context data")
	}
}

func TestAddRouteToContext(t *testing.T) {
	expectedRoute := "/expected/route"

	ctx := AddRouteToContext(context.Background(), expectedRoute)

	if gotData, ok := ctx.Value(contextDataKey).(*contextData); ok && gotData != nil {
		if gotData.route != expectedRoute {
			t.Errorf("Did not retrieve the desired route. Expected: %s; Got: %s", expectedRoute, gotData.route)
		}
	} else {
		t.Error("failed to retrieve context data")
	}
}
