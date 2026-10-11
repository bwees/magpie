package db

import (
	"context"

	"gorm.io/gorm"
)

type commitHooks struct {
	fns []func()
}

func (h *commitHooks) run() {
	for _, fn := range h.fns {
		fn()
	}
}

type commitHooksKey struct{}

func withCommitHooks(ctx context.Context, hooks *commitHooks) context.Context {
	return context.WithValue(ctx, commitHooksKey{}, hooks)
}

// AfterCommit runs fn once tx commits. A rolled-back transaction never runs it.
// fn must not block: it runs while the next writer waits.
func AfterCommit(tx *gorm.DB, fn func()) error {
	hooks, ok := tx.Statement.Context.Value(commitHooksKey{}).(*commitHooks)
	if !ok {
		return ErrNotInWrite
	}
	hooks.fns = append(hooks.fns, fn)
	return nil
}
