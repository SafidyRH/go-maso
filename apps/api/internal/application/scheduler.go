package application

import "context"

type MonitorScheduler interface {
	Sync(ctx context.Context, app Application) error
	Remove(ctx context.Context, applicationID string) error
}
