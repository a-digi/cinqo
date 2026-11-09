/***Statement***/
UPDATE media_files SET content_type = 'application/pdf' WHERE content_type = 'application/octet-stream' AND lower(extension) = '.pdf'
