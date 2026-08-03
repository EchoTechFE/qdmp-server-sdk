package qdmp_test

// Contract exercised by this file:
//
// Every business operation takes the caller's credential as an explicit
// per-call qdmp.Context (there is no token-bound client any more). Two
// properties must hold for ALL of them, not just the handful that happen to
// have a hand-written success test:
//
//  1. the token from the passed-in Context — and nothing else — is what ends
//     up in the outgoing auth header, in the header its authScheme dictates
//     (`access-token` for "standard", `x-openapi-access-token` for "genai");
//  2. an empty Context fails locally with ErrAccessTokenRequired and sends
//     no request at all.
//
// A per-operation table is the only thing that actually catches the failure
// modes of the WithAccessToken removal: an operation that forgot its
// requireAccessToken guard, one that reads the token from somewhere other
// than its qdmpCtx argument, or one that writes it into the wrong header.
// Spot-checking one method per group cannot see any of those.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// authHeaderStandard/authHeaderGenai are the two header names request.go
// selects between via authScheme.
const (
	authHeaderStandard = "access-token"
	authHeaderGenai    = "x-openapi-access-token"
)

// routeMetaPath is the single source of truth for every operation's method,
// path, authScheme and tokenRequired flag — generated from
// shared/openapi.yaml and checked for drift by the generate-check CI job.
const routeMetaPath = "../shared/generated/route-meta.json"

type routeMetaEntry struct {
	OperationID   string `json:"operationId"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	AuthScheme    string `json:"authScheme"`
	TokenRequired bool   `json:"tokenRequired"`
}

// loadTokenRequiredRoutes returns every operation route-meta marks as
// requiring a caller credential, keyed by operationId.
func loadTokenRequiredRoutes(t *testing.T) map[string]routeMetaEntry {
	t.Helper()
	raw, err := os.ReadFile(routeMetaPath)
	if err != nil {
		t.Fatalf("read %s: %v", routeMetaPath, err)
	}
	var all []routeMetaEntry
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatalf("parse %s: %v", routeMetaPath, err)
	}
	out := make(map[string]routeMetaEntry)
	for _, e := range all {
		if e.TokenRequired {
			out[e.OperationID] = e
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s contains no token-required operations; the matrix would vacuously pass", routeMetaPath)
	}
	return out
}

// expectedAuthHeader derives the header a given authScheme must carry the
// credential in, so the matrix never hardcodes that mapping per operation.
func expectedAuthHeader(t *testing.T, scheme string) string {
	t.Helper()
	switch scheme {
	case "standard":
		return authHeaderStandard
	case "genai":
		return authHeaderGenai
	default:
		t.Fatalf("route-meta declares authScheme %q for a token-required operation, "+
			"which this SDK has no auth header mapping for", scheme)
		return ""
	}
}

// businessOp is one row of the matrix: a public business method reduced to
// "call it with this credential". Everything else it is checked against —
// expected auth header, HTTP method and path — is read from route-meta, not
// restated here, so a route-meta change can never leave a stale expectation
// silently green.
type businessOp struct {
	operationID string
	call        func(c *qdmp.Client, ctx context.Context, qdmpCtx qdmp.Context) error
}

func allBusinessOps() []businessOp {
	strptr := func(s string) *string { return &s }
	return []businessOp{
		{"userMe", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.User.Me(ctx, q)
			return err
		}},
		{"islandDetail", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Island.Detail(ctx, q, generated.IslandDetailParams{Id: "island-1"})
			return err
		}},
		{"spuDetail", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Spu.Detail(ctx, q, generated.SpuDetailParams{Id: "spu-1"})
			return err
		}},
		{"spuSearch", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Spu.Search(ctx, q, generated.SpuSearchParams{
				Keyword: strptr("k"), Limit: strptr("20"), Offset: strptr("0"),
			})
			return err
		}},
		{"tagDetail", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Tag.Detail(ctx, q, generated.TagDetailParams{Id: "tag-1"})
			return err
		}},
		{"tagSearch", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Tag.Search(ctx, q, generated.TagSearchParams{
				Keyword: strptr("k"), Limit: strptr("20"), Offset: strptr("0"),
			})
			return err
		}},
		{"markAdd", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.Add(ctx, q, generated.MarkAddJSONBody{SpuId: "123"})
			return err
		}},
		{"markList", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.List(ctx, q, generated.MarkListParams{Limit: "20", Offset: "0"})
			return err
		}},
		{"markSearch", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.Search(ctx, q, generated.MarkSearchParams{Limit: "20", Offset: "0"})
			return err
		}},
		{"markDetail", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.Detail(ctx, q, generated.MarkDetailParams{Id: "mark-1", Limit: "20", Offset: "0"})
			return err
		}},
		{"wishAdd", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.WishSpu.Add(ctx, q, generated.WishAddJSONBody{Ids: []string{"1"}, Type: "SPU"})
			return err
		}},
		{"wishCancel", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.WishSpu.Cancel(ctx, q, generated.WishCancelJSONBody{Ids: []string{"1"}, Type: "SPU"})
			return err
		}},
		{"wishList", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.WishSpu.List(ctx, q, generated.WishListParams{Limit: "20", Offset: "0"})
			return err
		}},
		{"genaiGenerate", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.GenAI.Generate(ctx, q, generated.GenaiGenerateJSONBody{})
			return err
		}},
		{"genaiDetail", func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.GenAI.Detail(ctx, q, generated.GenaiDetailParams{Id: "task-1"})
			return err
		}},
	}
}

// TestBusinessOps_MatrixCoversEveryTokenRequiredRoute is what keeps the two
// tests below honest. Without it the table is just a hand-written copy of
// the routing rules, so adding an operation to shared/openapi.yaml — or
// flipping one between the standard and genai schemes — would leave both the
// implementation and the matrix stale and green at the same time. Asserting
// set equality against route-meta means any such change breaks this test
// until the matrix is updated too.
func TestBusinessOps_MatrixCoversEveryTokenRequiredRoute(t *testing.T) {
	routes := loadTokenRequiredRoutes(t)

	inMatrix := make(map[string]bool, len(allBusinessOps()))
	var duplicates []string
	for _, op := range allBusinessOps() {
		if inMatrix[op.operationID] {
			duplicates = append(duplicates, op.operationID)
		}
		inMatrix[op.operationID] = true
	}
	if len(duplicates) > 0 {
		t.Fatalf("matrix lists these operations more than once: %v", duplicates)
	}

	var missing, unknown []string
	for id := range routes {
		if !inMatrix[id] {
			missing = append(missing, id)
		}
	}
	for id := range inMatrix {
		if _, ok := routes[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 {
		t.Errorf("%s marks these operations tokenRequired but the matrix does not exercise them: %v",
			routeMetaPath, missing)
	}
	if len(unknown) > 0 {
		t.Errorf("the matrix exercises these operations, but %s does not list them as tokenRequired: %v",
			routeMetaPath, unknown)
	}
}

// TestBusinessOps_TokenComesFromPerCallContext gives every operation a token
// unique to that operation and asserts the server saw exactly that value in
// exactly the header its authScheme mandates — with the header, HTTP method
// and path all read from route-meta rather than restated here. A method that
// ignored its qdmpCtx, wrote the token into the other scheme's header, or
// called the wrong endpoint fails here.
func TestBusinessOps_TokenComesFromPerCallContext(t *testing.T) {
	routes := loadTokenRequiredRoutes(t)

	for _, op := range allBusinessOps() {
		t.Run(op.operationID, func(t *testing.T) {
			route, ok := routes[op.operationID]
			if !ok {
				t.Fatalf("%s is not a tokenRequired operation in %s", op.operationID, routeMetaPath)
			}
			wantHeader := expectedAuthHeader(t, route.AuthScheme)
			token := "at_token_for_" + op.operationID

			var mu sync.Mutex
			var gotStandard, gotGenai, gotMethod, gotPath string
			counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				gotStandard = r.Header.Get(authHeaderStandard)
				gotGenai = r.Header.Get(authHeaderGenai)
				gotMethod, gotPath = r.Method, r.URL.Path
				mu.Unlock()
				businessEnvelope(w, http.StatusOK, "0", "ok", "req-1", map[string]any{})
			})
			srv := startServer(t, counter)
			client := newTestClient(t, srv.URL)

			if err := op.call(client, context.Background(), qdmp.Context{AccessToken: token}); err != nil {
				t.Fatalf("%s error = %v, want nil", op.operationID, err)
			}
			if counter.Count() != 1 {
				t.Fatalf("%s hit the server %d times, want 1", op.operationID, counter.Count())
			}

			mu.Lock()
			standard, genai, method, path := gotStandard, gotGenai, gotMethod, gotPath
			mu.Unlock()

			// The table row must actually call the endpoint route-meta says it
			// does, otherwise a mis-wired row could satisfy the header check
			// while exercising a different operation entirely.
			if method != route.Method || path != route.Path {
				t.Fatalf("%s called %s %s, want %s %s per %s",
					op.operationID, method, path, route.Method, route.Path, routeMetaPath)
			}

			seen := map[string]string{authHeaderStandard: standard, authHeaderGenai: genai}
			if seen[wantHeader] != token {
				t.Fatalf("%s (authScheme %q): header %s = %q, want the per-call Context token %q",
					op.operationID, route.AuthScheme, wantHeader, seen[wantHeader], token)
			}
			// The scheme it does NOT use must not carry the credential too.
			other := authHeaderGenai
			if wantHeader == authHeaderGenai {
				other = authHeaderStandard
			}
			if seen[other] != "" {
				t.Fatalf("%s: token also leaked into %s = %q; only %s belongs to authScheme %q",
					op.operationID, other, seen[other], wantHeader, route.AuthScheme)
			}
		})
	}
}

// TestBusinessOps_EmptyContextFailsLocally asserts the fail-fast contract for
// every operation: a zero-value qdmp.Context must produce
// ErrAccessTokenRequired without any outbound request. This is the property
// that replaced "you cannot construct a caller without a token", so it has to
// hold for all 15 operations, not some of them.
func TestBusinessOps_EmptyContextFailsLocally(t *testing.T) {
	for _, op := range allBusinessOps() {
		t.Run(op.operationID, func(t *testing.T) {
			counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("%s sent a request despite an empty access token", op.operationID)
				businessEnvelope(w, http.StatusOK, "0", "ok", "req-1", map[string]any{})
			})
			srv := startServer(t, counter)
			client := newTestClient(t, srv.URL)

			err := op.call(client, context.Background(), qdmp.Context{})
			if err == nil {
				t.Fatalf("%s error = nil, want ErrAccessTokenRequired", op.operationID)
			}
			if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
				t.Fatalf("%s error = %v, want errors.Is(..., ErrAccessTokenRequired)", op.operationID, err)
			}
			if errors.Is(err, qdmp.ErrInvalidAccessToken) {
				t.Fatalf("%s: a missing token must not be reported as ErrInvalidAccessToken (err = %v)",
					op.operationID, err)
			}
			if counter.Count() != 0 {
				t.Fatalf("%s hit the server %d times, want 0", op.operationID, counter.Count())
			}
		})
	}
}
