package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// MCPBridge handles Model Context Protocol integration with Ollama
type MCPBridge struct {
	ollamaService *OllamaService
	tools         map[string]Tool
	mu            sync.RWMutex
	context       context.Context
	cancel        context.CancelFunc
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Handler     func(args map[string]interface{}) (string, error)
}

// ToolCall represents a tool execution request
type ToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Args   map[string]interface{} `json:"arguments"`
	Result string                 `json:"result,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

// MCPResponse represents a response from MCP tools
type MCPResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// NewMCPBridge creates a new MCP bridge
func NewMCPBridge(ollamaService *OllamaService) *MCPBridge {
	ctx, cancel := context.WithCancel(context.Background())
	
	bridge := &MCPBridge{
		ollamaService: ollamaService,
		tools:         make(map[string]Tool),
		context:       ctx,
		cancel:        cancel,
	}
	
	// Register built-in tools
	bridge.registerBuiltinTools()
	
	return bridge
}

// registerBuiltinTools registers the built-in Claude Code-like tools
func (m *MCPBridge) registerBuiltinTools() {
	// File system tools
	m.RegisterTool(Tool{
		Name:        "read_file",
		Description: "Read the contents of a file",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to read",
				},
			},
			"required": []string{"file_path"},
		},
		Handler: m.readFile,
	})
	
	m.RegisterTool(Tool{
		Name:        "write_file",
		Description: "Write content to a file",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			"required": []string{"file_path", "content"},
		},
		Handler: m.writeFile,
	})
	
	m.RegisterTool(Tool{
		Name:        "list_files",
		Description: "List files and directories in a path",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The directory path to list",
				},
			},
			"required": []string{"path"},
		},
		Handler: m.listFiles,
	})
	
	m.RegisterTool(Tool{
		Name:        "create_directory",
		Description: "Create a new directory",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The directory path to create",
				},
			},
			"required": []string{"path"},
		},
		Handler: m.createDirectory,
	})
	
	// Code execution tools
	m.RegisterTool(Tool{
		Name:        "execute_bash",
		Description: "Execute a bash command",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The bash command to execute",
				},
				"working_dir": map[string]interface{}{
					"type":        "string",
					"description": "The working directory for the command",
				},
			},
			"required": []string{"command"},
		},
		Handler: m.executeBash,
	})
	
	m.RegisterTool(Tool{
		Name:        "execute_python",
		Description: "Execute Python code",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"code": map[string]interface{}{
					"type":        "string",
					"description": "The Python code to execute",
				},
			},
			"required": []string{"code"},
		},
		Handler: m.executePython,
	})
	
	// Search and analysis tools
	m.RegisterTool(Tool{
		Name:        "search_files",
		Description: "Search for text patterns in files",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The pattern to search for",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The directory to search in",
				},
				"file_pattern": map[string]interface{}{
					"type":        "string",
					"description": "File pattern to match (e.g., '*.go', '*.py')",
				},
			},
			"required": []string{"pattern", "path"},
		},
		Handler: m.searchFiles,
	})
	
	// Git tools
	m.RegisterTool(Tool{
		Name:        "git_status",
		Description: "Get Git repository status",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The repository path",
				},
			},
		},
		Handler: m.gitStatus,
	})
	
	// System information tools
	m.RegisterTool(Tool{
		Name:        "get_system_info",
		Description: "Get system information",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{},
		},
		Handler: m.getSystemInfo,
	})
}

// RegisterTool registers a new tool
func (m *MCPBridge) RegisterTool(tool Tool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[tool.Name] = tool
}

// GetTools returns all registered tools
func (m *MCPBridge) GetTools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var tools []Tool
	for _, tool := range m.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ChatWithTools sends a chat message that can use tools
func (m *MCPBridge) ChatWithTools(prompt string) (*MCPResponse, error) {
	// Create system message with tool definitions
	systemMessage := m.createToolSystemMessage()
	
	// Combine system message with user prompt
	fullPrompt := fmt.Sprintf("%s\n\nUser: %s", systemMessage, prompt)
	
	// Send to Ollama
	response, err := m.ollamaService.Chat(fullPrompt)
	if err != nil {
		return nil, err
	}
	
	// Parse response for tool calls
	return m.parseToolResponse(response)
}

// createToolSystemMessage creates the system message with tool definitions
func (m *MCPBridge) createToolSystemMessage() string {
	var systemMsg strings.Builder
	
	systemMsg.WriteString("You are a helpful AI assistant with access to tools. ")
	systemMsg.WriteString("When you need to use a tool, respond with a JSON object in this format:\n")
	systemMsg.WriteString("```json\n")
	systemMsg.WriteString("{\n")
	systemMsg.WriteString("  \"tool_calls\": [\n")
	systemMsg.WriteString("    {\n")
	systemMsg.WriteString("      \"name\": \"tool_name\",\n")
	systemMsg.WriteString("      \"arguments\": {\"arg1\": \"value1\"}\n")
	systemMsg.WriteString("    }\n")
	systemMsg.WriteString("  ]\n")
	systemMsg.WriteString("}\n")
	systemMsg.WriteString("```\n\n")
	systemMsg.WriteString("Available tools:\n")
	
	for _, tool := range m.GetTools() {
		systemMsg.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description))
	}
	
	return systemMsg.String()
}

// parseToolResponse parses the response for tool calls and executes them
func (m *MCPBridge) parseToolResponse(response string) (*MCPResponse, error) {
	mcpResp := &MCPResponse{Content: response}
	
	// Look for JSON tool calls in the response
	if strings.Contains(response, "tool_calls") {
		// Extract JSON from code blocks
		jsonStart := strings.Index(response, "```json")
		if jsonStart != -1 {
			jsonStart += 7 // Skip ```json
			jsonEnd := strings.Index(response[jsonStart:], "```")
			if jsonEnd != -1 {
				jsonStr := response[jsonStart : jsonStart+jsonEnd]
				
				var toolRequest struct {
					ToolCalls []struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments"`
					} `json:"tool_calls"`
				}
				
				if err := json.Unmarshal([]byte(jsonStr), &toolRequest); err == nil {
					// Execute tool calls
					for i, tc := range toolRequest.ToolCalls {
						toolCall := ToolCall{
							ID:   fmt.Sprintf("call_%d", i),
							Name: tc.Name,
							Args: tc.Arguments,
						}
						
						if tool, exists := m.tools[tc.Name]; exists {
							result, err := tool.Handler(tc.Arguments)
							if err != nil {
								toolCall.Error = err.Error()
							} else {
								toolCall.Result = result
							}
						} else {
							toolCall.Error = "Tool not found"
						}
						
						mcpResp.ToolCalls = append(mcpResp.ToolCalls, toolCall)
					}
					
					// Generate follow-up response with tool results
					if len(mcpResp.ToolCalls) > 0 {
						followUpPrompt := m.createFollowUpPrompt(response, mcpResp.ToolCalls)
						followUpResponse, err := m.ollamaService.Chat(followUpPrompt)
						if err == nil {
							mcpResp.Content = followUpResponse
						}
					}
				}
			}
		}
	}
	
	return mcpResp, nil
}

// createFollowUpPrompt creates a follow-up prompt with tool results
func (m *MCPBridge) createFollowUpPrompt(originalResponse string, toolCalls []ToolCall) string {
	var prompt strings.Builder
	
	prompt.WriteString("Tool execution results:\n\n")
	
	for _, tc := range toolCalls {
		prompt.WriteString(fmt.Sprintf("Tool: %s\n", tc.Name))
		if tc.Error != "" {
			prompt.WriteString(fmt.Sprintf("Error: %s\n", tc.Error))
		} else {
			prompt.WriteString(fmt.Sprintf("Result: %s\n", tc.Result))
		}
		prompt.WriteString("\n")
	}
	
	prompt.WriteString("Please provide a helpful response based on these tool results.")
	
	return prompt.String()
}

// Tool handler implementations
func (m *MCPBridge) readFile(args map[string]interface{}) (string, error) {
	filePath, ok := args["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("file_path must be a string")
	}
	
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	
	return string(content), nil
}

func (m *MCPBridge) writeFile(args map[string]interface{}) (string, error) {
	filePath, ok := args["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("file_path must be a string")
	}
	
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content must be a string")
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filePath), nil
}

func (m *MCPBridge) listFiles(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}
	
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}
	
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Contents of %s:\n", path))
	
	for _, entry := range entries {
		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("📁 %s/\n", entry.Name()))
		} else {
			info, err := entry.Info()
			if err == nil {
				result.WriteString(fmt.Sprintf("📄 %s (%d bytes)\n", entry.Name(), info.Size()))
			} else {
				result.WriteString(fmt.Sprintf("📄 %s\n", entry.Name()))
			}
		}
	}
	
	return result.String(), nil
}

func (m *MCPBridge) createDirectory(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}
	
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	
	return fmt.Sprintf("Successfully created directory: %s", path), nil
}

func (m *MCPBridge) executeBash(args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("command must be a string")
	}
	
	workingDir := "."
	if wd, exists := args["working_dir"].(string); exists {
		workingDir = wd
	}
	
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = workingDir
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	
	result := fmt.Sprintf("Command: %s\nWorking Dir: %s\n\n", command, workingDir)
	
	if stdout.Len() > 0 {
		result += fmt.Sprintf("STDOUT:\n%s\n", stdout.String())
	}
	
	if stderr.Len() > 0 {
		result += fmt.Sprintf("STDERR:\n%s\n", stderr.String())
	}
	
	if err != nil {
		result += fmt.Sprintf("Exit Error: %v\n", err)
	} else {
		result += "Exit Code: 0\n"
	}
	
	return result, nil
}

func (m *MCPBridge) executePython(args map[string]interface{}) (string, error) {
	code, ok := args["code"].(string)
	if !ok {
		return "", fmt.Errorf("code must be a string")
	}
	
	// Create temporary Python file
	tmpFile, err := os.CreateTemp("", "trms_python_*.py")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	
	_, err = tmpFile.WriteString(code)
	if err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write code: %w", err)
	}
	tmpFile.Close()
	
	// Execute Python code
	cmd := exec.Command("python3", tmpFile.Name())
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err = cmd.Run()
	
	result := fmt.Sprintf("Python Code:\n%s\n\n", code)
	
	if stdout.Len() > 0 {
		result += fmt.Sprintf("OUTPUT:\n%s\n", stdout.String())
	}
	
	if stderr.Len() > 0 {
		result += fmt.Sprintf("ERROR:\n%s\n", stderr.String())
	}
	
	if err != nil {
		result += fmt.Sprintf("Exit Error: %v\n", err)
	}
	
	return result, nil
}

func (m *MCPBridge) searchFiles(args map[string]interface{}) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok {
		return "", fmt.Errorf("pattern must be a string")
	}
	
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}
	
	filePattern := "*"
	if fp, exists := args["file_pattern"].(string); exists {
		filePattern = fp
	}
	
	// Use ripgrep if available, otherwise use grep
	var cmd *exec.Cmd
	if _, err := exec.LookPath("rg"); err == nil {
		cmd = exec.Command("rg", "--type-add", fmt.Sprintf("custom:%s", filePattern), "-t", "custom", pattern, path)
	} else {
		cmd = exec.Command("grep", "-r", "--include", filePattern, pattern, path)
	}
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	
	return string(output), nil
}

func (m *MCPBridge) gitStatus(args map[string]interface{}) (string, error) {
	path := "."
	if p, exists := args["path"].(string); exists {
		path = p
	}
	
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	
	return string(output), nil
}

func (m *MCPBridge) getSystemInfo(args map[string]interface{}) (string, error) {
	info, err := GetSystemInfo()
	if err != nil {
		return "", fmt.Errorf("failed to get system info: %w", err)
	}
	
	result := fmt.Sprintf("System Information:\n")
	result += fmt.Sprintf("- Total Memory: %s\n", FormatMemory(info.TotalMemory))
	result += fmt.Sprintf("- Available Memory: %s\n", FormatMemory(info.AvailableMemory))
	result += fmt.Sprintf("- OS: %s\n", info.OS)
	result += fmt.Sprintf("- Architecture: %s\n", info.Arch)
	
	return result, nil
}

// Close shuts down the MCP bridge
func (m *MCPBridge) Close() {
	m.cancel()
}