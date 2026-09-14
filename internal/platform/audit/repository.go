package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type dbPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type postgresRepository struct {
	db dbPool
}

type Repository interface {
	Record(ctx context.Context, event Event) error
}

// NewPostgresRepository adapta o pool Postgres ao contrato de auditoria.
func NewPostgresRepository(db dbPool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Record(ctx context.Context, event Event) error {
	event.Code = strings.TrimSpace(event.Code)
	if event.Code == "" {
		return fmt.Errorf("audit record: event code vazio")
	}

	result := event.Result
	if result == "" {
		result = ResultSuccess
	}

	_, err := r.db.Exec(ctx, `
		call auditoria.sp_events_insert($1, $2, $3, $4, $5, $6, $7, $8);
	`,
		event.Code,
		nullableInt64(event.UserID),
		nullableString(event.IP),
		nullableString(event.Operation),
		nullableString(event.Object),
		nullableString(event.Module),
		nullableString(event.Message),
		nullableString(result),
	)
	if err != nil {
		return fmt.Errorf("audit record: %w", err)
	}
	return nil
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
