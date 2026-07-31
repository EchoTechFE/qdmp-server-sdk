// Package qdmp is the Go Server SDK for 千岛 (qdmp) OpenAPI.
//
// Construct a client with NewClient, exchange an app-level access token via
// Client.Auth.GetAccessToken (cached and single-flight refreshed
// automatically), and derive a per-user client with Client.WithAccessToken
// to call the business operation groups (User, Island, Spu, Tag, Mark,
// WishSpu, GenAI). See the exported types on Client, AuthService,
// QdmpApiError, and TokenStore for the full contract; the *_test.go files in
// this directory exercise it end-to-end against mock HTTP servers.
package qdmp
