# Actor-to-Actor Collaboration Protocol

This document defines the formal protocol for autonomous actors to collaborate within the Alloy kernel using the `IntentBroker`.

## 1. Intent Delegation (`intent:delegate`)

The primary mechanism for one actor to assign a task to another is the `intent:delegate` intent.

### Creation
An actor (the **Owner**) dispatches an `intent:delegate` to the `IntentBroker`. 

```json
{
  "name": "intent:delegate",
  "sender": "actor:agentA",
  "payload": {
    "id": "task-uuid",
    "parent_id": "parent-task-uuid", // Optional, for sub-tasks
    "assignee": "actor:agentB",
    "task": "Perform security audit on pkg/kernel",
    "payload": { ... task specific data ... }
  }
}
```

### Routing
The `IntentBroker` records the delegation and routes it to the `assignee`. If the `assignee` is empty, the broker routes it based on intent registrations matching the `task` description or specific capability requests.

## 2. Status Lifecycle

Assignees MUST update the broker on task progress using the following intents:

- `intent:delegate:update`: For incremental progress.
- `intent:delegate:complete`: Upon successful completion.
- `intent:delegate:failed`: If the task cannot be completed.

All update intents MUST include the task `id` and SHOULD include a `payload` with current results and an `attestation`.

## 3. Capability Discovery

Actors can discover providers for specific intents using `intent:query:providers`.

```json
{
  "name": "intent:query:providers",
  "payload": { "intent": "ai:summarize" }
}
```

The broker responds with a list of plugin/actor IDs that registered for that intent.

## 4. Verification Chain

Multi-step tasks are tracked via the `parent_id` and `chain` fields. An actor can query the full state of a task and its children using `intent:delegate:status` with the `deep: true` flag.

```json
{
  "name": "intent:delegate:status",
  "payload": { "id": "parent-task-uuid", "deep": true }
}
```

The broker will return the parent delegation along with the current state of all sub-tasks in the `sub_tasks` field.
