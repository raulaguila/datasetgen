-- Email outbox, background jobs, shared rate limit counters and the cleanup jobs.

-- The backend inserts pending rows; the mailer claims them with SKIP LOCKED.
-- available_at is the next eligible instant: backoff after a failure, or the end
-- of the claim lease. claim_id fences a worker whose lease ran out. The token in
-- payload is always sealed and leaves the row when the message finishes.
CREATE TABLE sys.outbox (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    purpose      TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending',
    attempts     INT         NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claim_id     UUID        NULL,
    request_id   TEXT        NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ NULL,
    last_error   TEXT        NULL,
    CONSTRAINT chk_sys_outbox_purpose CHECK (purpose IN ('activation', 'password_reset')),
    CONSTRAINT chk_sys_outbox_status CHECK (status IN ('pending', 'processing', 'sent', 'failed'))
);

CREATE INDEX idx_sys_outbox_claimable ON sys.outbox (available_at) WHERE status IN ('pending', 'processing');

-- Mail delivery cannot read or change users, passwords, roles or products.
GRANT SELECT ON sys.outbox TO dataset_mailer;
GRANT UPDATE (status, available_at, claim_id, processed_at, last_error, payload, attempts)
    ON sys.outbox TO dataset_mailer;

-- Background jobs for any module. A use case enqueues a row in its own
-- transaction, so the job exists exactly when the change it follows commits.
-- The backend's job worker claims due rows with SKIP LOCKED, the same lease and
-- claim_id fencing as sys.outbox. Email stays in sys.outbox: its mailer role
-- must not read anything else.
CREATE TABLE sys.job (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         TEXT        NOT NULL,
    payload      JSONB       NOT NULL DEFAULT '{}',
    status       TEXT        NOT NULL DEFAULT 'pending',
    attempts     INT         NOT NULL DEFAULT 0,
    max_attempts INT         NOT NULL DEFAULT 10,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claim_id     UUID        NULL,
    request_id   TEXT        NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ NULL,
    last_error   TEXT        NULL,
    CONSTRAINT chk_sys_job_kind CHECK (char_length(kind) BETWEEN 1 AND 100),
    CONSTRAINT chk_sys_job_status CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    CONSTRAINT chk_sys_job_max_attempts CHECK (max_attempts BETWEEN 1 AND 100)
);

CREATE INDEX idx_sys_job_claimable ON sys.job (available_at) WHERE status IN ('pending', 'processing');

-- One row per client and minute; UNLOGGED, since losing counts on a crash is fine.
CREATE UNLOGGED TABLE sys.rate_limit (
    key          TEXT        NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    hits         INT         NOT NULL DEFAULT 1,
    PRIMARY KEY (key, window_start)
);

-- Scheduled through pg_cron; without it, call it externally.
CREATE FUNCTION sys.fn_cleanup(retention INTERVAL DEFAULT INTERVAL '30 days')
RETURNS void
LANGUAGE sql
AS $$
  DELETE FROM usr.usr_login_attempt WHERE created_at < now() - retention;
  DELETE FROM usr.usr_session WHERE expires_at < now() - retention OR revoked_at < now() - retention;
  DELETE FROM usr.usr_token WHERE expires_at < now() - retention;
  DELETE FROM usr.usr_notification WHERE read_at < now() - retention OR created_at < now() - retention * 3;
  DELETE FROM sys.outbox WHERE status IN ('sent', 'failed') AND processed_at < now() - retention;
  DELETE FROM sys.job WHERE status IN ('done', 'failed') AND processed_at < now() - retention;
  DELETE FROM sys.rate_limit WHERE window_start < now() - INTERVAL '1 minute';
  -- Credential and tokens cascade. A live token means a resumed signup: it stays.
  DELETE FROM usr.usr_user u
   WHERE u.status = 'pending'
     AND u.created_at < now() - retention
     AND NOT EXISTS (SELECT 1 FROM usr.usr_token t WHERE t.user_id = u.id AND t.expires_at >= now());
$$;

-- pg_cron only exists in the project image, and only in cron.database_name.
DO $$
BEGIN
  IF current_setting('shared_preload_libraries', true) LIKE '%pg_cron%'
     AND current_setting('cron.database_name', true) = current_database() THEN
    CREATE EXTENSION IF NOT EXISTS pg_cron;
    PERFORM cron.schedule('dataset_cleanup', '17 3 * * *', 'SELECT sys.fn_cleanup()');
    PERFORM cron.schedule('dataset_rate_limit', '*/5 * * * *',
      $q$DELETE FROM sys.rate_limit WHERE window_start < now() - INTERVAL '1 minute'$q$);
  END IF;
END;
$$;
