-- Store Toggl projects for reference data
CREATE TABLE IF NOT EXISTS toggl_projects (
  id BIGINT PRIMARY KEY,
  workspace_id BIGINT NOT NULL,
  name TEXT NOT NULL,
  active TINYINT(1) NOT NULL,
  is_private TINYINT(1) NOT NULL,
  color VARCHAR(32) NOT NULL,
  client_id BIGINT NULL,
  at DATETIME(6) NOT NULL
) ENGINE=InnoDB;

