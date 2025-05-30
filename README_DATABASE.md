# Database Setup

This project now uses PostgreSQL with pgvector for persistent chat history and model improvement features.

## Quick Start

1. **Start the database:**
   ```bash
   docker-compose up -d postgres
   ```

2. **Verify database is running:**
   ```bash
   docker-compose ps
   ```

3. **Run the application:**
   ```bash
   go run .
   ```

## Features

### Chat Session Management
- Multiple chat sessions with different models
- Persistent chat history across restarts
- Easy switching between conversations

### Navigation
- **Tab**: Cycle modes (Command → AI → Chats → Models)
- **Chat Mode**: Browse and select different chat sessions
- **AI Mode**: Chat with models while preserving history

### Database Schema

**Chat Sessions:**
- ID, name, model name, timestamps
- Track which model each conversation uses

**Messages:**
- User and assistant messages
- Linked to chat sessions
- Vector embeddings for future AI improvements

**Model Feedback:**
- Rate responses (1-5 stars)
- Provide feedback for model improvement

## Database Management

**Connect to database:**
```bash
docker exec -it trms-postgres-1 psql -U trms -d trms
```

**View chat sessions:**
```sql
SELECT * FROM chat_sessions ORDER BY updated_at DESC;
```

**View recent messages:**
```sql
SELECT cs.name, m.role, m.content, m.created_at 
FROM messages m 
JOIN chat_sessions cs ON m.session_id = cs.id 
ORDER BY m.created_at DESC 
LIMIT 10;
```

**Create new chat session:**
```sql
INSERT INTO chat_sessions (name, model_name) VALUES ('My New Chat', 'llama2');
```

## Vector Features (Future)

The database includes pgvector support for:
- Semantic search across chat history
- Model response improvement
- Similar conversation discovery
- Context-aware responses

Vector embeddings will be generated using Ollama's embedding models and stored for enhanced AI interactions.

## Troubleshooting

**Database not connecting:**
```bash
# Restart database
docker-compose restart postgres

# Check logs
docker-compose logs postgres
```

**Reset database:**
```bash
# Stop and remove
docker-compose down -v

# Start fresh
docker-compose up -d postgres
```