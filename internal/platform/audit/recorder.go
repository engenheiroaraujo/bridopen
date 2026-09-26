package audit

import (
	"context"
	"log/slog"
)

// Recorder é a porta de aplicação para gravar auditoria sem acoplar handlers ao Postgres.
type Recorder interface {
	Record(ctx context.Context, event Event)
	RecordSuccess(ctx context.Context, userID int64, eventCode, entityType string, entityID int64)
	RecordFailure(ctx context.Context, userID int64, eventCode, entityType string, entityID int64)
}

type recorder struct {
	repo Repository
}

type noopRecorder struct{}

// NewRecorder cria um gravador; repo nil vira no-op (útil em testes).
func NewRecorder(repo Repository) Recorder {
	if repo == nil {
		return noopRecorder{}
	}
	return &recorder{repo: repo}
}

func (r recorder) Record(ctx context.Context, event Event) {
	if err := r.repo.Record(ctx, event); err != nil {
		slog.Warn("falha ao gravar auditoria",
			slog.String("event_code", event.Code),
			slog.String("err", err.Error()),
		)
	}
}

func (r recorder) RecordSuccess(ctx context.Context, userID int64, eventCode, entityType string, entityID int64) {
	r.Record(ctx, NewEvent(userID, eventCode, entityType, entityID, ResultSuccess))
}

func (r recorder) RecordFailure(ctx context.Context, userID int64, eventCode, entityType string, entityID int64) {
	r.Record(ctx, NewEvent(userID, eventCode, entityType, entityID, ResultFailure))
}

func (noopRecorder) Record(context.Context, Event) {}

func (noopRecorder) RecordSuccess(ctx context.Context, userID int64, eventCode, entityType string, entityID int64) {
}

func (noopRecorder) RecordFailure(ctx context.Context, userID int64, eventCode, entityType string, entityID int64) {
}
