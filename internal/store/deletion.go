package store

import (
	"errors"
)

// ErrModelNotManual is returned when deleting a model that is not manual.
// Discovered models cannot be deleted: the next discovery sync would
// resurrect them from the hub listing.
var ErrModelNotManual = errors.New("only manual models can be deleted")

// DeleteEndpoint removes an endpoint together with everything keyed by it:
// probe history, hourly rollups, the rollup watermark, and alert events. A
// probe round in flight for it finishes harmlessly: its record lands with a
// dangling endpoint_id (foreign keys are not enforced) and every HTTP read
// path existence-checks the endpoint first, so it stays invisible.
func (db *DB) DeleteEndpoint(id int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM alert_events WHERE endpoint_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM probes WHERE endpoint_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM probe_rollups WHERE endpoint_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM rollup_watermarks WHERE endpoint_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM endpoints WHERE id = ?", id); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteModel removes a manual model with its endpoints and everything keyed
// by them (probes, rollups, watermarks, alert events). It returns
// ErrModelNotManual for discovered models. Past eval results are kept: they
// denormalize model_id text so eval history still renders after the model is
// gone. Note: if the model_id is still present in the hub's /v1/models
// listing, the next discovery sync re-registers it as a discovered model
// with fresh endpoints; the UI warns about this in the confirm dialog.
func (db *DB) DeleteModel(id int64) error {
	model, err := db.GetModel(id)
	if err != nil {
		return err
	}
	if model.Origin != "manual" {
		return ErrModelNotManual
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		"DELETE FROM alert_events WHERE endpoint_id IN (SELECT id FROM endpoints WHERE model_id = ?)", id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"DELETE FROM probes WHERE endpoint_id IN (SELECT id FROM endpoints WHERE model_id = ?)", id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"DELETE FROM probe_rollups WHERE endpoint_id IN (SELECT id FROM endpoints WHERE model_id = ?)", id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"DELETE FROM rollup_watermarks WHERE endpoint_id IN (SELECT id FROM endpoints WHERE model_id = ?)", id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM endpoints WHERE model_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM models WHERE id = ?", id); err != nil {
		return err
	}

	return tx.Commit()
}
