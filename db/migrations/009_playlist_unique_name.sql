-- Ensure unique playlist names per user
ALTER TABLE playlists ADD CONSTRAINT unique_user_playlist_name UNIQUE (user_id, name);
