//go:build lintfixture

// Package crosscontext must fail lint: it reaches into a context past its api package.
// `mise run lint:boundaries` checks that depguard rejects it.
package crosscontext

import "magpie/internal/catalog"

var _ = catalog.Module
