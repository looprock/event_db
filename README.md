# Example API

A simple API that receives event data and stores it in a PostgreSQL database.

## Setup

1. Clone the repository
2. Set up your environment variables (see Configuration section)
3. Run database migrations:
   ```bash
   go run scripts/migrate.go
   ```
4. Start the server:
   ```bash
   go run cmd/server/main.go
   ```

## Configuration

The application uses environment variables for configuration:

```bash
# Server Configuration
PORT=8081           # Port for the API server (default: 8081)
API_TOKEN=secret    # Required authentication token
DB_PATH=./data/events.db  # Database path (default: ./data/events.db)
```

## API Endpoints

### POST /api/events

Receives and stores event data.

**Headers:**
- `Authorization`: API token (required)
- `Content-Type`: application/json

**Request Body:**
```json
{
  "tags": ["word1", "word2", "..."],
  "body": "event body content"
}
```

**Response:**
```json
{
  "id": 1,
  "tags": ["word1", "word2", "..."],
  "body": "event body content",
  "created_at": "2024-04-25T20:48:34Z"
}
```

### GET /api/events?tag=word
Returns all events with the given tag.

### GET /api/events/by-date?date=YYYY-MM-DD
Returns all events created on the given date.

### GET /api/events/:id
Returns a single event by ID.

## Database Schema

The application uses PostgreSQL with the following schema:

```sql
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tags TEXT NOT NULL,  -- JSON array of tags
    body TEXT NOT NULL,
    source TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Development

### Running Migrations

The application uses SQL migrations to manage the database schema. Migrations are stored in the `migrations` directory and are executed in order based on their filename prefix.

To run migrations:
```bash
go run scripts/migrate.go
```

### Adding New Migrations

1. Create a new SQL file in the `migrations` directory
2. Name it with a sequential number prefix (e.g., `002_add_new_field.sql`)
3. Write your SQL statements
4. Run the migration script

## License

MIT 