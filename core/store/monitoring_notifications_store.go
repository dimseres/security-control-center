package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *monitoringStore) ListNotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, name, telegram_bot_token, telegram_chat_id, telegram_thread_id, template_text, quiet_hours_enabled, quiet_hours_start, quiet_hours_end, quiet_hours_tz, silent, protect_content, is_default, created_by, created_at, is_active
		FROM notification_channels
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []NotificationChannel
	for rows.Next() {
		ch, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, *ch)
	}
	return res, rows.Err()
}

func (s *monitoringStore) GetNotificationChannel(ctx context.Context, id int64) (*NotificationChannel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, type, name, telegram_bot_token, telegram_chat_id, telegram_thread_id, template_text, quiet_hours_enabled, quiet_hours_start, quiet_hours_end, quiet_hours_tz, silent, protect_content, is_default, created_by, created_at, is_active
		FROM notification_channels WHERE id=?`, id)
	return scanNotificationChannel(row)
}

func (s *monitoringStore) CreateNotificationChannel(ctx context.Context, ch *NotificationChannel) (int64, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	if ch.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE notification_channels SET is_default=0`); err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO notification_channels(type, name, telegram_bot_token, telegram_chat_id, telegram_thread_id, template_text, quiet_hours_enabled, quiet_hours_start, quiet_hours_end, quiet_hours_tz, silent, protect_content, is_default, created_by, created_at, is_active)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		strings.ToLower(strings.TrimSpace(ch.Type)), strings.TrimSpace(ch.Name), ch.TelegramBotTokenEnc,
		strings.TrimSpace(ch.TelegramChatID), nullableID(ch.TelegramThreadID), strings.TrimSpace(ch.TemplateText), boolToInt(ch.QuietHoursEnabled),
		strings.TrimSpace(ch.QuietHoursStart), strings.TrimSpace(ch.QuietHoursEnd), strings.TrimSpace(ch.QuietHoursTZ),
		boolToInt(ch.Silent), boolToInt(ch.ProtectContent), boolToInt(ch.IsDefault), ch.CreatedBy, now, boolToInt(ch.IsActive))
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	ch.ID = id
	ch.CreatedAt = now
	return id, nil
}

func (s *monitoringStore) UpdateNotificationChannel(ctx context.Context, ch *NotificationChannel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if ch.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE notification_channels SET is_default=0`); err != nil {
			tx.Rollback()
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE notification_channels
		SET type=?, name=?, telegram_bot_token=?, telegram_chat_id=?, telegram_thread_id=?, template_text=?, quiet_hours_enabled=?, quiet_hours_start=?, quiet_hours_end=?, quiet_hours_tz=?, silent=?, protect_content=?, is_default=?, is_active=?
		WHERE id=?`,
		strings.ToLower(strings.TrimSpace(ch.Type)), strings.TrimSpace(ch.Name), ch.TelegramBotTokenEnc,
		strings.TrimSpace(ch.TelegramChatID), nullableID(ch.TelegramThreadID), strings.TrimSpace(ch.TemplateText), boolToInt(ch.QuietHoursEnabled),
		strings.TrimSpace(ch.QuietHoursStart), strings.TrimSpace(ch.QuietHoursEnd), strings.TrimSpace(ch.QuietHoursTZ),
		boolToInt(ch.Silent), boolToInt(ch.ProtectContent), boolToInt(ch.IsDefault), boolToInt(ch.IsActive), ch.ID)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *monitoringStore) DeleteNotificationChannel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notification_channels WHERE id=?`, id)
	return err
}

func (s *monitoringStore) ListMonitorNotifications(ctx context.Context, monitorID int64) ([]MonitorNotification, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, monitor_id, notification_channel_id, enabled
		FROM monitor_notifications WHERE monitor_id=?
		ORDER BY id`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []MonitorNotification
	for rows.Next() {
		var item MonitorNotification
		var enabled int
		if err := rows.Scan(&item.ID, &item.MonitorID, &item.NotificationChannelID, &enabled); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		res = append(res, item)
	}
	return res, rows.Err()
}

func (s *monitoringStore) ReplaceMonitorNotifications(ctx context.Context, monitorID int64, items []MonitorNotification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM monitor_notifications WHERE monitor_id=?`, monitorID); err != nil {
		tx.Rollback()
		return err
	}
	for _, item := range items {
		if item.NotificationChannelID == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO monitor_notifications(monitor_id, notification_channel_id, enabled)
			VALUES(?,?,?)`, monitorID, item.NotificationChannelID, boolToInt(item.Enabled)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *monitoringStore) ListDefaultNotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, name, telegram_bot_token, telegram_chat_id, telegram_thread_id, template_text, quiet_hours_enabled, quiet_hours_start, quiet_hours_end, quiet_hours_tz, silent, protect_content, is_default, created_by, created_at, is_active
		FROM notification_channels WHERE is_default=1 AND is_active=1
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []NotificationChannel
	for rows.Next() {
		ch, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, *ch)
	}
	return res, rows.Err()
}

func (s *monitoringStore) GetNotificationState(ctx context.Context, monitorID int64) (*MonitorNotificationState, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT monitor_id, last_notified_at, last_down_notified_at, last_up_notified_at, last_tls_notified_at, last_maintenance_notified_at, down_started_at, down_sequence
		FROM monitor_notification_state WHERE monitor_id=?`, monitorID)
	var st MonitorNotificationState
	var lastNotified, lastDown, lastUp, lastTLS, lastMaint, downStarted sql.NullTime
	if err := row.Scan(&st.MonitorID, &lastNotified, &lastDown, &lastUp, &lastTLS, &lastMaint, &downStarted, &st.DownSequence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastNotified.Valid {
		st.LastNotifiedAt = &lastNotified.Time
	}
	if lastDown.Valid {
		st.LastDownNotifiedAt = &lastDown.Time
	}
	if lastUp.Valid {
		st.LastUpNotifiedAt = &lastUp.Time
	}
	if lastTLS.Valid {
		st.LastTLSNotifiedAt = &lastTLS.Time
	}
	if lastMaint.Valid {
		st.LastMaintenanceNotifiedAt = &lastMaint.Time
	}
	if downStarted.Valid {
		st.DownStartedAt = &downStarted.Time
	}
	return &st, nil
}

func (s *monitoringStore) UpsertNotificationState(ctx context.Context, st *MonitorNotificationState) error {
	if st == nil {
		return nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE monitor_notification_state
		SET last_notified_at=?, last_down_notified_at=?, last_up_notified_at=?, last_tls_notified_at=?, last_maintenance_notified_at=?, down_started_at=?, down_sequence=?
		WHERE monitor_id=?`,
		nullableTime(st.LastNotifiedAt), nullableTime(st.LastDownNotifiedAt), nullableTime(st.LastUpNotifiedAt), nullableTime(st.LastTLSNotifiedAt),
		nullableTime(st.LastMaintenanceNotifiedAt), nullableTime(st.DownStartedAt), st.DownSequence, st.MonitorID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO monitor_notification_state(monitor_id, last_notified_at, last_down_notified_at, last_up_notified_at, last_tls_notified_at, last_maintenance_notified_at, down_started_at, down_sequence)
		VALUES(?,?,?,?,?,?,?,?)`,
		st.MonitorID, nullableTime(st.LastNotifiedAt), nullableTime(st.LastDownNotifiedAt), nullableTime(st.LastUpNotifiedAt),
		nullableTime(st.LastTLSNotifiedAt), nullableTime(st.LastMaintenanceNotifiedAt), nullableTime(st.DownStartedAt), st.DownSequence)
	return err
}

func scanNotificationChannel(row interface {
	Scan(dest ...any) error
}) (*NotificationChannel, error) {
	var ch NotificationChannel
	var threadID sql.NullInt64
	var silent, protect, def, active, quietEnabled int
	if err := row.Scan(&ch.ID, &ch.Type, &ch.Name, &ch.TelegramBotTokenEnc, &ch.TelegramChatID, &threadID, &ch.TemplateText, &quietEnabled, &ch.QuietHoursStart, &ch.QuietHoursEnd, &ch.QuietHoursTZ, &silent, &protect, &def, &ch.CreatedBy, &ch.CreatedAt, &active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if threadID.Valid {
		ch.TelegramThreadID = &threadID.Int64
	}
	ch.Silent = silent == 1
	ch.ProtectContent = protect == 1
	ch.IsDefault = def == 1
	ch.IsActive = active == 1
	ch.QuietHoursEnabled = quietEnabled == 1
	return &ch, nil
}

func (s *monitoringStore) ListNotificationDeliveries(ctx context.Context, limit int) ([]MonitorNotificationDelivery, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, monitor_id, notification_channel_id, event_type, status, error_text, body_preview, created_at, acknowledged_at, acknowledged_by
		FROM monitor_notification_deliveries
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]MonitorNotificationDelivery, 0, limit)
	for rows.Next() {
		var item MonitorNotificationDelivery
		var monitorID, ackBy sql.NullInt64
		var ackAt sql.NullTime
		if err := rows.Scan(&item.ID, &monitorID, &item.NotificationChannelID, &item.EventType, &item.Status, &item.Error, &item.BodyPreview, &item.CreatedAt, &ackAt, &ackBy); err != nil {
			return nil, err
		}
		if monitorID.Valid {
			val := monitorID.Int64
			item.MonitorID = &val
		}
		if ackAt.Valid {
			item.AcknowledgedAt = &ackAt.Time
		}
		if ackBy.Valid {
			val := ackBy.Int64
			item.AcknowledgedBy = &val
		}
		res = append(res, item)
	}
	return res, rows.Err()
}

func (s *monitoringStore) AddNotificationDelivery(ctx context.Context, item *MonitorNotificationDelivery) (int64, error) {
	if item == nil {
		return 0, errors.New("nil delivery")
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO monitor_notification_deliveries(monitor_id, notification_channel_id, event_type, status, error_text, body_preview, created_at, acknowledged_at, acknowledged_by)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		nullableID(item.MonitorID), item.NotificationChannelID, strings.TrimSpace(item.EventType), strings.TrimSpace(item.Status), strings.TrimSpace(item.Error), strings.TrimSpace(item.BodyPreview), item.CreatedAt, nullableTime(item.AcknowledgedAt), nullableID(item.AcknowledgedBy))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	item.ID = id
	return id, nil
}

func (s *monitoringStore) AcknowledgeNotificationDelivery(ctx context.Context, id int64, userID int64) error {
	if id == 0 || userID == 0 {
		return errors.New("invalid id")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE monitor_notification_deliveries
		SET acknowledged_at=?, acknowledged_by=?
		WHERE id=? AND acknowledged_at IS NULL`, time.Now().UTC(), userID, id)
	return err
}
