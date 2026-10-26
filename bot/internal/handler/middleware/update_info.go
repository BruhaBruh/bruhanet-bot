package middleware

import (
	"context"
	"sync"
)

type infoKey struct{}

type UpdateInfo struct {
	mu       sync.Mutex
	route    string
	answered bool
	panicked bool
}

func withInfo(ctx context.Context, info *UpdateInfo) context.Context {
	return context.WithValue(ctx, infoKey{}, info)
}

func UpdateInfoFrom(ctx context.Context) *UpdateInfo {
	info, _ := ctx.Value(infoKey{}).(*UpdateInfo)
	return info
}

func (i *UpdateInfo) SetRoute(route string) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.route = route
}

func (i *UpdateInfo) Route() string {
	if i == nil {
		return ""
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.route
}

func (i *UpdateInfo) MarkAnswered() {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.answered = true
}

func (i *UpdateInfo) Answered() bool {
	if i == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.answered
}

func (i *UpdateInfo) MarkPanicked() {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.panicked = true
}

func (i *UpdateInfo) Panicked() bool {
	if i == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.panicked
}
