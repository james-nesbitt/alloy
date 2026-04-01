//go:build wasip1 || wasm

package main

import (
	. "github.com/james-nesbitt/alloy/pkg/wasm/guest"
	"encoding/json"
	"fmt"
	"strings"
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
		SetBackground(true).
		WithMetadata(
			"Task Manager (Actor)",
			"Manages tasks and automatically scans buffers for TODOs",
			"0.2.0",
			"Alloy Team",
		).
		WithTags("tasks", "productivity", "todo", "actor").
		WithCapability("tasks:create", "Create a new task").
		WithCapability("tasks:list", "List all tasks").
		WithCapability("tasks:scan", "Manually scan a buffer for tasks")

	// Set up message handlers
	plugin.Handle("tasks:create", handleCreate)
	plugin.Handle("tasks:list", handleList)
	plugin.Handle("tasks:scan", handleScan)
	plugin.Handle("buffer:update", handleBufferUpdate)

	// Backward compatibility handlers
	plugin.Handle("create", handleCreate)
	plugin.Handle("list", handleList)

	// Set up initialization
	plugin.OnInit(func() error {
		plugin.Log("info", "Tasks actor initializing")

		// Subscribe to buffer updates to automatically scan for TODOs
		subPayload, _ := json.Marshal(map[string]string{"topic": "buffer:update"})
		plugin.RouteMessage(AlloyMessage{
			Id:      "sub-buffer-updates",
			MsgType: "request",
			Sender:  "tasks",
			Target:  Some("events"),
			Method:  "subscribe",
			Payload: subPayload,
		})

		return nil
	})

	// Run the plugin
	if err := plugin.Run(); err != nil {
		plugin.Log("error", "Plugin failed: "+err.Error())
	}
}

func handleBufferUpdate(msg AlloyMessage) AlloyMessage {
	var event struct {
		BufferID  string `json:"buffer_id"`
		Event     string `json:"event"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return AlloyMessage{}
	}

	// We only care about content updates
	if event.Event != "update" && event.Event != "create" && event.Event != "append" {
		return AlloyMessage{}
	}

	// Trigger a scan of the buffer
	scanPayload, _ := json.Marshal(map[string]string{"id": event.BufferID})
	plugin.RouteMessage(AlloyMessage{
		Id:      fmt.Sprintf("auto-scan-%s-%d", event.BufferID, time.Now().Unix()),
		MsgType: "request",
		Method:  "tasks:scan",
		Sender:  "tasks",
		Target:  Some("tasks"),
		Payload: scanPayload,
	})

	return AlloyMessage{}
}

func handleScan(msg AlloyMessage) AlloyMessage {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return plugin.ErrorReply(msg, "invalid_request")
	}

	// Read the buffer content
	buffer, ok := plugin.ReadBuffer(req.ID)
	if !ok {
		return plugin.ErrorReply(msg, "buffer_not_found")
	}

	content := string(buffer.Content)
	lines := strings.Split(content, "\n")
	foundCount := 0

	for i, line := range lines {
		upper := strings.ToUpper(line)
		idx := strings.Index(upper, "TODO:")
		if idx != -1 {
			title := strings.TrimSpace(line[idx+5:])
			if title == "" {
				continue
			}

			// Check if we already have this task (simplified check)
			exists := false
			for _, t := range tasks {
				if t.Title == title {
					exists = true
					break
				}
			}

			if !exists {
				taskID := fmt.Sprintf("task-auto-%d", time.Now().UnixNano())
				task := Task{
					ID:          taskID,
					Title:       title,
					Description: fmt.Sprintf("Auto-discovered in %s line %d", buffer.Name, i+1),
					Status:      "pending",
					CreatedAt:   time.Now().Unix(),
					UpdatedAt:   time.Now().Unix(),
				}
				tasks[taskID] = task
				foundCount++
				plugin.Log("info", "Auto-discovered task: "+title)
			}
		}
	}

	return plugin.Reply(msg, map[string]interface{}{
		"status": "scanned",
		"found":  foundCount,
	})
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
