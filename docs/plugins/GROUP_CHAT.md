# Group Chat Plugin (`chat`)

The Group Chat plugin provides real-time communication, direct messaging, and presence tracking.

## Methods

- `send`: Send a message to a public channel.
- `history`: Retrieve message history for a channel.
- `direct:send`: Send a private message to another user.
- `direct:history`: Retrieve private message history between two users.
- `presence:update`: Update your presence status (e.g., online, away, offline).
- `presence:list`: List all online users and their status.
- `ping`: Check plugin availability.

## Events Published

- `chat:message`: Emitted when a message is sent to a channel.
- `chat:direct`: Emitted when a direct message is sent.
- `chat:presence`: Emitted when a user updates their presence.

## Storage (KV Store)

- `history:<channel>`: JSON array of `ChatMessage`.
- `dm:<user1>:<user2>`: JSON array of `DirectMessage` (sender/receiver sorted alphabetically).
- `presence:list`: JSON map of `Presence` objects keyed by username.
