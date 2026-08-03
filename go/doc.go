// Package qdmp is the Go Server SDK for 千岛小程序开放平台 OpenAPI.
//
// Construct a client with NewClient, exchange an app-level access token via
// Client.Auth.GetAppAccessToken (cached and single-flight refreshed
// automatically), and call the business operation groups (User, Island, Spu,
// Tag, Mark, WishSpu, GenAI) on it, passing the caller's credential
// explicitly on every call as a Context. See the exported types on Client, AuthService,
// QdmpApiError, and TokenStore for the full contract; the *_test.go files in
// this directory exercise it end-to-end against mock HTTP servers.
package qdmp
