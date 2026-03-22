//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"time"
)

// Task represents a task in the system.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"` // pending, in-progress, completed
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// TaskCreateRequest represents a request to create a task.
type TaskCreateRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// TaskCreateResponse represents a response to creating a task.
type TaskCreateResponse struct {
	Status string `json:"status"`
	Task   Task   `json:"task"`
}

// TaskListResponse represents a response to listing tasks.
type TaskListResponse struct {
	Tasks []Task `json:"tasks"`
}

var (
	plugin *Plugin
	tasks  = make(map[string]Task)
)

func main() {
	// Create a new WIT-based plugin
	plugin = NewPlugin("tasks").
		WithMetadata(
			"Task Manager", 
			"Manages tasks and to-do items for the system",
			"0.1.0", 
			"Alloy Team",
		).
		WithTags("tasks", "productivity", "todo").
		WithCapability("create", "Create a new task").
		WithCapability("list", "List all tasks")

	// Set up message handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("list", handleList)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Tasks plugin initializing")
		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

// handleCreate handles task creation requests.
func handleCreate(msg AlloyMessage) AlloyMessage {
	var req TaskCreateRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request: "+err.Error())
	}

	// Create the task
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	task := Task{
		ID:          taskID,
		Title:       req.Title,
		Description: req.Description,
		Status:      "pending",
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	tasks[taskID] = task

	plugin.Log("info", "Created task: "+task.Title)

	return plugin.Reply(msg, TaskCreateResponse{
		Status: "created",
		Task:   task,
	})
}

// handleList handles task list requests.
func handleList(msg AlloyMessage) AlloyMessage {
	// Convert tasks map to slice
	taskList := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		taskList = append(taskList, task)
	}

	return plugin.Reply(msg, TaskListResponse{
		Tasks: taskList,
	})
}
