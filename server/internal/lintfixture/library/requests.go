//go:build lintfixture

// Package libraryfixture must fail lint: Library neither reads Requests nor consumes its events.
// `mise run lint:boundaries` checks that depguard rejects it.
package libraryfixture

import requestsapi "magpie/internal/requests/api"

var _ = requestsapi.Messages
