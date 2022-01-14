package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/calmitchell617/sqlpipe/internal/validator"
)

type Transfer struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	SourceID     int64     `json:"sourceID"`
	SourceName   string    `json:"sourceName"`
	TargetID     int64     `json:"targetID"`
	TargetName   string    `json:"targetName"`
	Query        string    `json:"query"`
	TargetSchema string    `json:"targetSchema"`
	TargetTable  string    `json:"targetTable"`
	Overwrite    bool      `json:"overwrite"`
	Status       string    `json:"status"`
	Error        string    `json:"error"`
	StoppedAt    time.Time `json:"stoppedAt"`
	Version      int       `json:"version"`
}

type TransferModel struct {
	DB *sql.DB
}

func (m TransferModel) Insert(transfer *Transfer) (*Transfer, error) {
	query := `
        INSERT INTO transfers (source_id, target_id, query, target_schema, target_table, overwrite, stopped_at) 
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, created_at, version`

	args := []interface{}{
		transfer.SourceID,
		transfer.TargetID,
		transfer.Query,
		transfer.TargetSchema,
		transfer.TargetTable,
		transfer.Overwrite,
		transfer.StoppedAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&transfer.ID, &transfer.CreatedAt, &transfer.Version)
	if err != nil {
		return transfer, err
	}

	return transfer, nil
}

func ValidateTransfer(v *validator.Validator, transfer *Transfer) {
	v.Check(transfer.SourceID != 0, "sourceId", "A Source ID is required")
	v.Check(transfer.TargetID != 0, "targetId", "A Target ID is required")
	v.Check(transfer.Query != "", "query", "A query is required")
	v.Check(transfer.TargetTable != "", "targetTable", "A target table is required")
}

func (m TransferModel) GetAll(filters Filters) ([]*Transfer, Metadata, error) {
	query := fmt.Sprintf(`
	SELECT
	count(*) OVER(),
	transfers.id,
	transfers.created_at,
	transfers.source_id,
	source.name,
	transfers.target_id,
	target.name,
	transfers.query,
	transfers.target_schema,
	transfers.target_table,
	transfers.overwrite,
	transfers.status,
	transfers.error,
	transfers.stopped_at,
	transfers.version
FROM
	transfers
left join
	connections source
on
	transfers.source_id = source.id
left join
	connections target
on
	transfers.source_id = target.id
order by
	%s %s,
	id asc
limit
	$1
offset
	$2
`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []interface{}{filters.limit(), filters.offset()}

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}

	defer rows.Close()

	totalRecords := 0
	transfers := []*Transfer{}

	for rows.Next() {
		var transfer Transfer

		err := rows.Scan(
			&totalRecords,
			&transfer.ID,
			&transfer.CreatedAt,
			&transfer.SourceID,
			&transfer.SourceName,
			&transfer.TargetID,
			&transfer.TargetName,
			&transfer.Query,
			&transfer.TargetSchema,
			&transfer.TargetTable,
			&transfer.Overwrite,
			&transfer.Status,
			&transfer.Error,
			&transfer.StoppedAt,
			&transfer.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		transfers = append(transfers, &transfer)
	}

	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return transfers, metadata, nil
}

func (m TransferModel) GetById(id int64) (*Transfer, error) {
	query := `
        SELECT id, created_at, source_id, target_id, query, target_schema, target_table, overwrite, status, error, stopped_at, version
        FROM transfers
        WHERE id = $1`

	var transfer Transfer

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&transfer.ID,
		&transfer.CreatedAt,
		&transfer.SourceID,
		&transfer.TargetID,
		&transfer.Query,
		&transfer.TargetSchema,
		&transfer.TargetTable,
		&transfer.Overwrite,
		&transfer.Status,
		&transfer.Error,
		&transfer.StoppedAt,
		&transfer.Version,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &transfer, nil
}

func (m TransferModel) Update(transfer *Transfer) error {
	query := `
        UPDATE transfers 
        SET status = $1, error = $2, stopped_at = $3, version = version + 1
        WHERE id = $4 AND version = $5
        RETURNING version`

	args := []interface{}{
		&transfer.Status,
		&transfer.Error,
		&transfer.StoppedAt,
		&transfer.ID,
		&transfer.Version,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&transfer.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (m TransferModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
			DELETE FROM transfers
			WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}
