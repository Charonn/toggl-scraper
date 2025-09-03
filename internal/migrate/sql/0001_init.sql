-- Create initial schema for Toggl time entries
CREATE TABLE IF NOT EXISTS toggl_time_entries (
  id BIGINT PRIMARY KEY,
  description TEXT,
  project_id BIGINT NULL,
  workspace_id BIGINT NULL,
  tags TEXT,
  start DATETIME(6) NOT NULL,
  stop DATETIME(6) NULL,
  duration_sec BIGINT NOT NULL
) ENGINE=InnoDB;

