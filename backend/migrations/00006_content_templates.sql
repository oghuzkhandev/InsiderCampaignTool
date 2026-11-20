-- +goose Up
-- Reusable text only: applying a template still creates a validated campaign content version.
CREATE TABLE content_templates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name ~ '[^ \t\n\r\f\v]' AND char_length(name) <= 100),
    sms_text text NOT NULL CHECK (content_text_valid(sms_text)),
    email_subject text NOT NULL CHECK (content_text_valid(email_subject)),
    email_body text NOT NULL CHECK (content_text_valid(email_body)),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE content_templates;
