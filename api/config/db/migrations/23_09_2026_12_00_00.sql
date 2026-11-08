/***Statement***/
UPDATE media_files SET title = original_filename WHERE title IS NULL OR title = '';
