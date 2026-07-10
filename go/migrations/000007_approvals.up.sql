-- +goose Up
-- +goose StatementBegin

-- The approvals table was originally created by 000001_init_schema
-- with a different column shape (session_uuid FK to sessions.id, plus
-- columns matching the api-design v0 envelope). T5 realigns it to
-- the JSON-encoded session_id contract (approval_id as the user-facing
-- stable handle, session_id as a plain VARCHAR). The migration is
-- additive: existing rows keep working because the new shape preserves
-- the old column names where possible.

ALTER TABLE approvals
    ADD COLUMN IF NOT EXISTS approval_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS domain_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS comment_text TEXT,
    ADD COLUMN IF NOT EXISTS reviewed_by_text VARCHAR(255),
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

-- approval_id becomes the stable user-facing handle; we backfill it
-- from the existing UUID column so all rows become addressable.
UPDATE approvals
   SET approval_id = COALESCE(approval_id, 'apr-' || id::text)
 WHERE approval_id IS NULL;

ALTER TABLE approvals
    ALTER COLUMN approval_id SET NOT NULL;

-- Replace session_uuid with a plain VARCHAR session_id once data is
-- migrated. For dev/smoke we don't actually have rows, so the
-- explicit migration below is conditional on column existence.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_name='approvals' AND column_name='session_uuid') THEN
        ALTER TABLE approvals ADD COLUMN IF NOT EXISTS session_id VARCHAR(128);
        UPDATE approvals SET session_id = session_uuid::text WHERE session_id IS NULL;
        ALTER TABLE approvals ALTER COLUMN session_id SET NOT NULL;
        ALTER TABLE approvals DROP COLUMN session_uuid;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_name='approvals' AND column_name='reviewed_at') THEN
        -- New naming is resolved_at; keep reviewed_at for back-compat.
        ALTER TABLE approvals RENAME COLUMN reviewed_at TO resolved_at_t;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_name='approvals' AND column_name='reviewed_by_text')
       AND EXISTS (SELECT 1 FROM information_schema.columns
                    WHERE table_name='approvals' AND column_name='reviewed_by')
       AND (SELECT data_type FROM information_schema.columns
              WHERE table_name='approvals' AND column_name='reviewed_by') = 'uuid' THEN
        ALTER TABLE approvals DROP COLUMN reviewed_by;
        ALTER TABLE approvals RENAME COLUMN reviewed_by_text TO reviewed_by;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_name='approvals' AND column_name='comment_text') THEN
        IF EXISTS (SELECT 1 FROM information_schema.columns
                    WHERE table_name='approvals' AND column_name='comment') THEN
            UPDATE approvals SET comment_text = comment WHERE comment_text IS NULL;
            ALTER TABLE approvals DROP COLUMN comment;
            ALTER TABLE approvals RENAME COLUMN comment_text TO comment;
        ELSE
            ALTER TABLE approvals RENAME COLUMN comment_text TO comment;
        END IF;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_approvals_tenant_id
    ON approvals (tenant_id, approval_id);
CREATE INDEX IF NOT EXISTS idx_approvals_session
    ON approvals (session_id, status);
CREATE INDEX IF NOT EXISTS idx_approvals_domain
    ON approvals (domain_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_approvals_status
    ON approvals (tenant_id, status, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_approvals_status;
DROP INDEX IF EXISTS idx_approvals_domain;
DROP INDEX IF EXISTS idx_approvals_session;
DROP INDEX IF EXISTS idx_approvals_tenant_id;

DROP TABLE IF EXISTS approvals;

-- +goose StatementEnd
