-- Update status check constraint to include 'deleted'
ALTER TABLE stories DROP CONSTRAINT stories_status_check;
ALTER TABLE stories ADD CONSTRAINT stories_status_check CHECK (status IN ('draft', 'published', 'deleted'));
