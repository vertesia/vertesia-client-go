// Package vertesia provides the Go SDK facade for the Vertesia API.
//
// The recommended entry point is NewClient. It configures the generated Studio
// and Store API clients, sets the x-api-version header, and handles either
// direct bearer tokens or sk- secret keys that are exchanged through STS.
//
// Use the fields on Client for the common SDK surface, such as AccountsAPI and
// ObjectsAPI. The underlying generated OpenAPI clients remain available through
// Client.Studio and Client.Store when advanced access is needed.
package vertesia
