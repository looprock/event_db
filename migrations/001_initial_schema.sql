-- Create events table
CREATE TABLE IF NOT EXISTS events (
    id SERIAL PRIMARY KEY,
    tags TEXT NOT NULL,  -- JSON array of tags
    body TEXT NOT NULL,
    source TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_tags ON events(tags); 

-- Create event_logs table
CREATE TABLE IF NOT EXISTS event_logs (
    id SERIAL PRIMARY KEY,
    event_id INTEGER NOT NULL,
    status TEXT NOT NULL,  -- 'success' or 'error'
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (event_id) REFERENCES events(id)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_event_logs_event_id ON event_logs(event_id);
CREATE INDEX IF NOT EXISTS idx_event_logs_status ON event_logs(status);
CREATE INDEX IF NOT EXISTS idx_event_logs_created_at ON event_logs(created_at); 