-- Create paragraph_images table
CREATE TABLE IF NOT EXISTS paragraph_images (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT  NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    image_url    TEXT NOT NULL,
    position     INT  NOT NULL DEFAULT 0
);

-- Migrate existing images if any
DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='paragraphs' AND column_name='image_url') THEN
        INSERT INTO paragraph_images (paragraph_id, image_url, position)
        SELECT id, image_url, 0 FROM paragraphs WHERE image_url IS NOT NULL AND image_url != '';
        
        ALTER TABLE paragraphs DROP COLUMN image_url;
    END IF;
END $$;
