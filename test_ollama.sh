#!/bin/bash

echo "Testing Ollama API..."

# Check if Ollama is running
if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
    echo "✓ Ollama is running"
else
    echo "✗ Ollama is not running"
    echo "  Please run: ollama serve"
    exit 1
fi

# Check if llama2 model exists
echo ""
echo "Checking for llama2 model..."
MODELS=$(curl -s http://localhost:11434/api/tags | jq -r '.models[].name' 2>/dev/null)

if echo "$MODELS" | grep -q "llama2"; then
    echo "✓ llama2 model is available"
else
    echo "✗ llama2 model not found"
    echo "  Please run: ollama pull llama2"
    exit 1
fi

# Test a simple chat
echo ""
echo "Testing chat with llama2..."
RESPONSE=$(curl -s http://localhost:11434/api/generate \
  -d '{
    "model": "llama2",
    "prompt": "Say hello in one word",
    "stream": false
  }' | jq -r '.response' 2>/dev/null)

if [ -n "$RESPONSE" ]; then
    echo "✓ Got response: $RESPONSE"
else
    echo "✗ No response from Ollama"
fi