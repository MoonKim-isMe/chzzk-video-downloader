package persistence

import (
	"fmt"
	"time"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
)

func (d *Database) UpsertDownloadTask(task downloader.DownloadTask) error {
	_, err := d.db.Exec(`INSERT INTO download_tasks (
		task_id,
		video_no,
		video_title,
		channel_name,
		thumbnail_image_url,
		url,
		output_dir,
		status,
		progress_status,
		progress_percent,
		downloaded_bytes,
		total_bytes,
		total_bytes_estimated,
		speed_bytes_per_second,
		eta_seconds,
		final_path,
		error,
		error_code,
		queued_at,
		started_at,
		finished_at,
		updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(task_id) DO UPDATE SET
		video_no = excluded.video_no,
		video_title = excluded.video_title,
		channel_name = excluded.channel_name,
		thumbnail_image_url = excluded.thumbnail_image_url,
		url = excluded.url,
		output_dir = excluded.output_dir,
		status = excluded.status,
		progress_status = excluded.progress_status,
		progress_percent = excluded.progress_percent,
		downloaded_bytes = excluded.downloaded_bytes,
		total_bytes = excluded.total_bytes,
		total_bytes_estimated = excluded.total_bytes_estimated,
		speed_bytes_per_second = excluded.speed_bytes_per_second,
		eta_seconds = excluded.eta_seconds,
		final_path = excluded.final_path,
		error = excluded.error,
		error_code = excluded.error_code,
		queued_at = excluded.queued_at,
		started_at = excluded.started_at,
		finished_at = excluded.finished_at,
		updated_at = excluded.updated_at`,
		task.TaskID,
		task.VideoNo,
		task.VideoTitle,
		task.ChannelName,
		task.ThumbnailImageURL,
		task.URL,
		task.OutputDir,
		string(task.Status),
		task.Progress.Status,
		task.Progress.Percent,
		task.Progress.DownloadedBytes,
		task.Progress.TotalBytes,
		boolInt(task.Progress.TotalBytesEstimated),
		task.Progress.SpeedBytesPerSecond,
		task.Progress.ETASeconds,
		task.FinalPath,
		task.Error,
		string(task.ErrorCode),
		task.QueuedAt,
		task.StartedAt,
		task.FinishedAt,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("다운로드 이력을 SQLite에 저장할 수 없습니다: %w", err)
	}
	return nil
}

func (d *Database) DeleteDownloadTask(taskID string) error {
	result, err := d.db.Exec(`DELETE FROM download_tasks WHERE task_id = ?`, taskID)
	if err != nil {
		return fmt.Errorf("다운로드 이력을 삭제할 수 없습니다: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("다운로드 이력 삭제 결과를 확인할 수 없습니다: %w", err)
	}
	return nil
}

func (d *Database) ListDownloadTasks() ([]downloader.DownloadTask, error) {
	rows, err := d.db.Query(`SELECT
		task_id,
		video_no,
		video_title,
		channel_name,
		thumbnail_image_url,
		url,
		output_dir,
		status,
		progress_status,
		progress_percent,
		downloaded_bytes,
		total_bytes,
		total_bytes_estimated,
		speed_bytes_per_second,
		eta_seconds,
		final_path,
		error,
		error_code,
		queued_at,
		started_at,
		finished_at
	FROM download_tasks
	ORDER BY queued_at, task_id`)
	if err != nil {
		return nil, fmt.Errorf("다운로드 이력을 조회할 수 없습니다: %w", err)
	}
	defer rows.Close()

	tasks := make([]downloader.DownloadTask, 0)
	for rows.Next() {
		var task downloader.DownloadTask
		var status string
		var estimated int
		if err := rows.Scan(
			&task.TaskID,
			&task.VideoNo,
			&task.VideoTitle,
			&task.ChannelName,
			&task.ThumbnailImageURL,
			&task.URL,
			&task.OutputDir,
			&status,
			&task.Progress.Status,
			&task.Progress.Percent,
			&task.Progress.DownloadedBytes,
			&task.Progress.TotalBytes,
			&estimated,
			&task.Progress.SpeedBytesPerSecond,
			&task.Progress.ETASeconds,
			&task.FinalPath,
			&task.Error,
			&task.ErrorCode,
			&task.QueuedAt,
			&task.StartedAt,
			&task.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("다운로드 이력 데이터를 읽을 수 없습니다: %w", err)
		}

		task.Status = downloader.TaskStatus(status)
		if !validTaskStatus(task.Status) {
			return nil, fmt.Errorf("저장된 다운로드 상태가 올바르지 않습니다: %s", status)
		}
		task.Progress.TotalBytesEstimated = scanBool(estimated)
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("다운로드 이력 조회를 완료할 수 없습니다: %w", err)
	}
	return tasks, nil
}

func (d *Database) RecoverInterruptedDownloads() error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := d.db.Exec(`UPDATE download_tasks
	SET
		status = ?,
		error_code = '',
		error = CASE
			WHEN error = '' THEN '앱 종료로 다운로드가 중단되었습니다.'
			ELSE error
		END,
		finished_at = CASE
			WHEN finished_at = '' THEN ?
			ELSE finished_at
		END,
		updated_at = ?
	WHERE status IN (?, ?)`,
		string(downloader.TaskStatusCancelled),
		now,
		now,
		string(downloader.TaskStatusQueued),
		string(downloader.TaskStatusRunning),
	)
	if err != nil {
		return fmt.Errorf("중단된 다운로드 이력을 복구할 수 없습니다: %w", err)
	}
	return nil
}

func validTaskStatus(status downloader.TaskStatus) bool {
	switch status {
	case downloader.TaskStatusQueued,
		downloader.TaskStatusRunning,
		downloader.TaskStatusCompleted,
		downloader.TaskStatusFailed,
		downloader.TaskStatusCancelled:
		return true
	default:
		return false
	}
}
