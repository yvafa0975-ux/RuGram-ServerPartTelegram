\set ON_ERROR_STOP on

-- main and v2 intentionally have independent migration histories. Keep them
-- in separate PostgreSQL databases even when they share the local container.
SELECT 'CREATE DATABASE rugram_main OWNER rugram'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'rugram_main')
\gexec

SELECT 'CREATE DATABASE rugram_v2 OWNER rugram'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'rugram_v2')
\gexec
