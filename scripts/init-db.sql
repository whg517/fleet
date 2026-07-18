-- Fleet: PostgreSQL init script
-- Creates the fleet database (already created via POSTGRES_DB, this is for extras)

-- Enable required extensions
\c fleet
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Note: Application schema is managed by ent atlas migrations
