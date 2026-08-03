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
	"errors"
	"net/http"
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

// businessOp is one row of the matrix: a public business method reduced to
// "call it with this credential", plus the auth header it is expected to
// use.
type businessOp struct {
	name   string
	header string
	call   func(c *qdmp.Client, ctx context.Context, qdmpCtx qdmp.Context) error
}

func allBusinessOps() []businessOp {
	strptr := func(s string) *string { return &s }
	return []businessOp{
		{"user.me", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.User.Me(ctx, q)
			return err
		}},
		{"island.detail", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Island.Detail(ctx, q, generated.IslandDetailParams{Id: "island-1"})
			return err
		}},
		{"spu.detail", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Spu.Detail(ctx, q, generated.SpuDetailParams{Id: "spu-1"})
			return err
		}},
		{"spu.search", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Spu.Search(ctx, q, generated.SpuSearchParams{
				Keyword: strptr("k"), Limit: strptr("20"), Offset: strptr("0"),
			})
			return err
		}},
		{"tag.detail", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Tag.Detail(ctx, q, generated.TagDetailParams{Id: "tag-1"})
			return err
		}},
		{"tag.search", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Tag.Search(ctx, q, generated.TagSearchParams{
				Keyword: strptr("k"), Limit: strptr("20"), Offset: strptr("0"),
			})
			return err
		}},
		{"mark.add", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.Add(ctx, q, generated.MarkAddJSONBody{SpuId: "123"})
			return err
		}},
		{"mark.list", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.List(ctx, q, generated.MarkListParams{Limit: "20", Offset: "0"})
			return err
		}},
		{"mark.search", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.Search(ctx, q, generated.MarkSearchParams{Limit: "20", Offset: "0"})
			return err
		}},
		{"mark.detail", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.Mark.Detail(ctx, q, generated.MarkDetailParams{Id: "mark-1", Limit: "20", Offset: "0"})
			return err
		}},
		{"wishspu.add", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.WishSpu.Add(ctx, q, generated.WishAddJSONBody{Ids: []string{"1"}, Type: "SPU"})
			return err
		}},
		{"wishspu.cancel", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.WishSpu.Cancel(ctx, q, generated.WishCancelJSONBody{Ids: []string{"1"}, Type: "SPU"})
			return err
		}},
		{"wishspu.list", authHeaderStandard, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.WishSpu.List(ctx, q, generated.WishListParams{Limit: "20", Offset: "0"})
			return err
		}},
		{"genai.generate", authHeaderGenai, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.GenAI.Generate(ctx, q, generated.GenaiGenerateJSONBody{})
			return err
		}},
		{"genai.detail", authHeaderGenai, func(c *qdmp.Client, ctx context.Context, q qdmp.Context) error {
			_, err := c.GenAI.Detail(ctx, q, generated.GenaiDetailParams{Id: "task-1"})
			return err
		}},
	}
}

// TestBusinessOps_TokenComesFromPerCallContext gives every operation a token
// unique to that operation and asserts the server saw exactly that value in
// exactly the header its authScheme mandates. A method that ignored its
// qdmpCtx (or wrote the token into the other scheme's header) fails here.
func TestBusinessOps_TokenComesFromPerCallContext(t *testing.T) {
	for _, op := range allBusinessOps() {
		t.Run(op.name, func(t *testing.T) {
			token := "at_token_for_" + op.name

			var mu sync.Mutex
			var gotStandard, gotGenai string
			counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				gotStandard = r.Header.Get(authHeaderStandard)
				gotGenai = r.Header.Get(authHeaderGenai)
				mu.Unlock()
				businessEnvelope(w, http.StatusOK, "0", "ok", "req-1", map[string]any{})
			})
			srv := startServer(t, counter)
			client := newTestClient(t, srv.URL)

			if err := op.call(client, context.Background(), qdmp.Context{AccessToken: token}); err != nil {
				t.Fatalf("%s error = %v, want nil", op.name, err)
			}
			if counter.Count() != 1 {
				t.Fatalf("%s hit the server %d times, want 1", op.name, counter.Count())
			}

			mu.Lock()
			standard, genai := gotStandard, gotGenai
			mu.Unlock()

			want := map[string]string{authHeaderStandard: standard, authHeaderGenai: genai}
			if want[op.header] != token {
				t.Fatalf("%s: header %s = %q, want the per-call Context token %q",
					op.name, op.header, want[op.header], token)
			}
			// The scheme it does NOT use must not carry the credential too.
			other := authHeaderGenai
			if op.header == authHeaderGenai {
				other = authHeaderStandard
			}
			if want[other] != "" {
				t.Fatalf("%s: token also leaked into %s = %q; only %s belongs to this authScheme",
					op.name, other, want[other], op.header)
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
		t.Run(op.name, func(t *testing.T) {
			counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("%s sent a request despite an empty access token", op.name)
				businessEnvelope(w, http.StatusOK, "0", "ok", "req-1", map[string]any{})
			})
			srv := startServer(t, counter)
			client := newTestClient(t, srv.URL)

			err := op.call(client, context.Background(), qdmp.Context{})
			if err == nil {
				t.Fatalf("%s error = nil, want ErrAccessTokenRequired", op.name)
			}
			if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
				t.Fatalf("%s error = %v, want errors.Is(..., ErrAccessTokenRequired)", op.name, err)
			}
			if errors.Is(err, qdmp.ErrInvalidAccessToken) {
				t.Fatalf("%s: a missing token must not be reported as ErrInvalidAccessToken (err = %v)",
					op.name, err)
			}
			if counter.Count() != 0 {
				t.Fatalf("%s hit the server %d times, want 0", op.name, counter.Count())
			}
		})
	}
}
