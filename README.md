# Kerykeion Terminal

Kerykeion Terminal (`kerxt`) is a terminal application for peer-to-peer chat and file transfers over a local network. Each running instance discovers peers, accepts connections, sends messages, and stores its own history. There is no central server or cloud account.

The application is written in Go. It uses Bubble Tea for the terminal interface, TLS over TCP for connections, UDP for discovery, and SQLite for local state.

The code is source-available under a custom license. Personal use, noncommercial education and research, and internal office or business use are permitted. Selling the application or providing paid access to it is not permitted. See [License](#license) for the terms.

## Project status

This is a development build. Local integration tests cover direct messages, room joining and history, direct transfers, and room downloads through `/get`. Broadcast discovery across physical networks and automatic reconnection after interrupted transfers still need work.

The protocol contains messages for some unfinished flows. Their presence in the code does not mean those features are ready for use across devices. See [Current limitations](#current-limitations) before testing on a shared network.

## Install

You need Git and a Go toolchain compatible with [go.mod](go.mod), which currently declares Go 1.27.3. The installer builds the application from source. It provides Go installation instructions if the compiler is missing.

After cloning or downloading this repository, run these commands from its root:

```bash
bash install.sh
kerxt
```

On Linux, the installer builds `kerxt`, sets executable permissions, and installs it at `~/.local/bin/kerxt`. It checks `PATH` and adds a shell configuration entry when needed. If it changes your shell configuration, follow the printed reload instructions or open a new terminal.

Installer and uninstaller messages are in English. The language setting applies to the TUI.

### Manual Linux build

Install Go and Git with your distribution's package manager:

```bash
# CachyOS / Arch Linux
sudo pacman -S go git

# Fedora
sudo dnf install -y golang git
```

Then build and install:

```bash
mkdir -p ~/.local/bin
go build -ldflags="-s -w" -o ~/.local/bin/kerxt ./cmd/kerxt
```

Make sure `~/.local/bin` is in your shell's `PATH`. To build and run without installing:

```bash
go build -o kerxt ./cmd/kerxt
./kerxt
```

### Cross-compilation

```bash
# Windows, amd64
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o kerxt.exe ./cmd/kerxt

# macOS, Apple Silicon
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o kerxt-darwin-arm64 ./cmd/kerxt
```

A successful cross-build does not verify runtime behavior, operating-system permissions, or firewall rules on the target machine.

### Uninstall

```bash
# Remove the binary from the default installation location
kerxt uninstall

# Also remove application configuration, history, cache, and downloaded files
kerxt uninstall --purge
```

## Use

```bash
kerxt                  # Open the TUI
kerxt join lounge      # Join or create a room at startup
kerxt --repl           # Use the line-based interface
kerxt doctor           # Print network diagnostics
```

Flags go before the subcommand. For example:

```bash
kerxt --config /tmp/kerxt-alice --name Alice --port 43100 --udp-port 43101
```

`--config` separates identity, history, configuration, and downloads from the default application directories (downloads use `<config-dir>/downloads` in this mode). Two instances using the same directory share the same identity and database; use separate directories for separate test peers.

### Panels and input focus

The TUI starts with the system panel. Opening a room or direct conversation adds the chat panel. The panels are stacked on smaller terminals and placed side by side when space permits. The sidebar is hidden below 100 columns.

The `>` marker in a panel title identifies the current input and scrolling focus.

| Focus | What Enter does |
|---|---|
| System panel | Executes input beginning with `/`; other text stays out of chat |
| Chat panel | Sends the text as a message, including text beginning with `/` |
| Chat minimized | Handles input as system commands |

After opening a chat, press F6 before entering a command such as `/who` or `/leave`. Otherwise that text is sent to the conversation.

| Key | Action |
|---|---|
| F2 | Switch between English and Indonesian; save the preference |
| F6 | Switch input and scrolling focus between system and chat |
| Esc | Cancel an open file picker; otherwise minimize chat |
| F4 | Restore minimized chat; do nothing if chat is already visible |
| PgUp / PgDn | Scroll the focused panel |
| Tab | Complete commands or room names while system input has focus |
| Ctrl+C | Quit |
| Ctrl+O | Open the file picker while chat has focus |
| Ctrl+Y | Open a stable list of user and file IDs for copying |

Minimizing chat does not leave the room or disconnect the peer. Messages received for the active conversation remain in its history, and a counter appears while the panel is hidden. F4 reopens it and clears that counter. The F4 hint is hidden while chat is visible.

Each panel has a scrollbar on its right edge. The thick segment shows the visible fraction of its history and moves with the reading position. It is an indicator, not a draggable control.

### Attaching files

To attach a file, focus a conversation and press Ctrl+O. The picker starts in your home directory and remembers the last folder for the current session. It lists one directory at a time, including hidden entries; it does not scan subfolders. The current path and recipient appear above the list.

| Picker key | Action |
|---|---|
| Up / Down | Select an entry |
| PgUp / PgDn | Move by a page |
| Enter / Right | Open a folder or select a file |
| Backspace / Left | Open the parent folder |
| Enter on confirmation | Send the selected file |
| Backspace on confirmation | Return to the file list |
| Esc | Cancel and return to chat |
| Ctrl+C | Quit the application |

Selecting a file shows its path, size, and recipient before sending. Esc preserves your chat draft and keeps the chat panel open. Files must be regular files within the configured size limit. Directory reads and send preparation run outside the UI event loop.

From the system panel, `/send` also accepts a complete path containing spaces, with or without surrounding quotes:

```text
/send ~/Downloads/Image.jpeg
/send "/home/user/Downloads/Image.jpeg"
```

Relative paths are resolved from the directory where `kerxt` was launched. `/send` starts sending immediately; it does not open the picker or request confirmation. The line-based interface (`--repl`) supports `/send` with the same path rules, but has no Ctrl+O picker. Paths are not executed as shell commands or expanded using shell variables.

Confirmed attachments appear in the DM or room history with their filename, size, and SHA-256 file ID. Room attachments also show the local `/get` number when their announcement is available.

Selecting a file for a room sends an announcement to its current members. Recipients download it with `/get <number>` from the system panel. Use the number displayed in your own application; announcement numbers are assigned locally. Completion shows the saved file path. Both peers need this version of the file-request protocol.

English is the default TUI language. Use F2, `/lang en`, or `/lang id` to change it. Run slash commands from the system panel.

### Peer identities and duplicate names

Usernames are display names and do not have to be unique. Each identity has a stable 64-character peer ID derived from its public key. Renaming a user keeps that ID; deleting `identity.key` and creating a new identity changes it.

The peer sidebar, DM title, and message authors show IDs alongside names. `/who` shows full IDs and selectors such as `Budi#12ab34cd`. Short IDs start at eight characters and grow when a longer prefix is needed to distinguish discovered peers.

From the system panel, these selectors all work:

```text
/dm Budi
/dm 12ab34cd
/dm Budi#12ab34cd
/dm Budi+12ab34cd
/dm Aria Putra
```

The IDs above are examples; copy an actual selector from `/who`. Full IDs are accepted too. A name alone works when it identifies one peer. Duplicate names and ambiguous ID prefixes show choices without changing the active conversation. A qualified selector must match both the name and ID. `/go` accepts the same peer selectors, while joined room names retain priority.

### Copying IDs

Press Ctrl+Y to open a snapshot of your ID, discovered peer IDs, and file IDs from loaded chat messages and announcements. Use Up/Down to select an entry and Enter to copy its complete ID, even when the displayed label is shortened. Esc returns to the previous focus and preserves the chat draft. Ctrl+C still quits the app.

The list stays unchanged while new messages and transfer updates arrive in the background. This also gives you a stable place to select the ID with the mouse and use your terminal's copy shortcut. Selecting text in the live TUI can be cleared when its contents redraw.

Native clipboard access uses Windows clipboard APIs, `pbcopy` on macOS, or Linux clipboard tools. Wayland needs `wl-copy` (`wl-clipboard`); X11 needs `xclip` or `xsel`. If clipboard access fails, the list remains open with an error so you can use terminal selection instead. The copy shortcut is available in the TUI, not the line-based REPL.

### Commands

| Command | Action |
|---|---|
| `/help [command]` | Show help, including subcommands such as `/help room transfer` |
| `/who` | Show your identity, discovered peer names, unique selectors, full IDs, and online/stale status |
| `/dm <name\|ID\|name#ID>` | Open a DM by name, full ID, unique ID prefix, or qualified name (`name+ID` also works) |
| `/join <room>` | Search for a room; create it if no room is found |
| `/room new <name>` | Create a room explicitly |
| `/room list` | List joined rooms and rooms found automatically on the LAN |
| `/room info [name]` | Show room metadata and members |
| `/room transfer <name> <peer>` | Transfer the master role |
| `/go <target>` | Open a joined room or a conversation with a discovered peer |
| `/send <path>` | Send immediately to the active DM or prepare a room announcement; spaces, surrounding quotes, and `~/` are supported |
| `/get <number\|file_ID>` | Download using a local announcement number or the complete file ID copied with Ctrl+Y; show the saved path |
| `/leave [room]` | Leave a room locally and notify online members |
| `/disband [room]` | Close a room as its master |
| `/name <new_name>` | Change the display name without replacing the identity |
| `/open <number>` | Open a registered HTTP or HTTPS link in the default browser |
| `/doctor` | Print interface, port, and troubleshooting information |
| `/add <ip:port>` | Send a discovery announcement to a specified host |
| `/lang <id|en>` | Set and save the TUI language |
| `/quit` | Quit from system input |

## Architecture

Each process runs the same components. The entry point in [cmd/kerxt/main.go](cmd/kerxt/main.go) creates them, registers callbacks, and routes incoming frames to the appropriate handler.

```mermaid
flowchart TB
    Main["cmd/kerxt: startup and frame dispatch"]
    UI["pkg/ui: TUI or REPL"]
    Discovery["pkg/discovery: UDP presence and room queries"]
    Messaging["pkg/messaging: messages, ACKs, outbox"]
    Room["pkg/room: membership and room events"]
    Transfer["pkg/transfer: file offers, chunks, bitmap"]
    Transport["pkg/transport: TCP listeners and TLS connections"]
    Protocol["pkg/protocol: frames and payloads"]
    Store["pkg/db: SQLite history and metadata"]
    Identity["pkg/identity: persistent Ed25519 keys"]
    Peers["Other LAN peers"]

    Main --> UI
    Main --> Discovery
    Main --> Transport
    UI --> Messaging
    UI --> Room
    UI --> Transfer
    Messaging --> Store
    Room --> Store
    Transfer --> Store
    Messaging --> Transport
    Room --> Transport
    Transfer --> Transport
    Transport --> Protocol
    Discovery --> Identity
    Messaging --> Identity
    Room --> Identity
    Transport --> Identity
    Discovery <-->|UDP| Peers
    Transport <-->|TCP + TLS| Peers
```

| Package | Responsibility |
|---|---|
| [`config`](pkg/config) | Defaults, OS-specific directories, saved settings |
| [`identity`](pkg/identity) | Key creation/loading, peer fingerprints, signatures |
| [`discovery`](pkg/discovery) | Announcements, peer addresses, presence timeouts, interface polling |
| [`protocol`](pkg/protocol) | Frame encoding, payload types, signature input formats |
| [`transport`](pkg/transport) | TLS certificates, listeners, connection maps, serialized frame writes |
| [`messaging`](pkg/messaging) | Signed messages, deduplication, acknowledgments, retries, link registration |
| [`room`](pkg/room) | Room creation, joining/leaving, master changes, tombstones, history exchange |
| [`transfer`](pkg/transfer) | Hashing, file offers, chunk storage, partial-file metadata, thumbnails |
| [`db`](pkg/db) | SQLite schema and persistence operations |
| [`ui`](pkg/ui) | Input routing, panes, language selection, message and progress rendering |
| [`doctor`](pkg/doctor) | Local interface and socket diagnostics |
| [`installer`](pkg/installer) | Binary installation, permissions, PATH setup, removal |

Discovery and data transfer use separate paths. UDP advertises where a peer can be reached. TCP carries chat, room events, and file data. File uploads open a connection separate from the chat connection so they do not share the same frame stream.

Default ports are TCP `42100` and UDP `42101`. The TCP listener tries up to ten consecutive ports if the requested port is occupied. UDP discovery uses the configured port directly.

TCP frames have a five-byte header:

```text
[1-byte type][4-byte payload length, big-endian][payload]
```

Control payloads are JSON. A file chunk contains a four-byte chunk index followed by binary data. Frames are limited to 1 MiB; file chunks are 256 KiB.

## Runtime flows

### Startup

1. Load defaults and saved configuration, then apply CLI overrides.
2. Load `identity.key`, or generate an Ed25519 key pair. Compute `peer_id` from the SHA-256 hash of the public key.
3. Open `history.db`, create missing tables, and run retention cleanup.
4. Start the TCP listener, message retry worker, file cleanup, and UDP discovery.
5. Register frame handlers and discovery callbacks, then start the TUI or REPL.

The identity is independent of the IP address. Moving between network interfaces does not intentionally create a new peer identity.

### Peer discovery

The discovery loop sends a signed `ANNOUNCE` every four seconds. It calculates IPv4 broadcast addresses from active interfaces, sends to a multicast target, and also sends unicast announcements to peers already held in memory. A separate loop checks interface changes every two seconds.

On receiving an announcement, the process checks its timestamp and signature, derives the sender's peer ID, and updates its address and last-seen time. A peer becomes stale after 15 seconds without an announcement. Discovery callbacks persist new or changed addresses and trigger message retries.

Sending to a multicast address does not establish multicast reception by itself. Broadcast availability, access-point isolation, and host firewalls still affect discovery.

### Direct message

```mermaid
sequenceDiagram
    participant UI as Sender UI
    participant Sender as Sender messaging
    participant DB as Sender SQLite
    participant Receiver as Receiver messaging

    UI->>Sender: Message body and target peer
    Sender->>DB: Save signed message as queued
    Sender->>Receiver: MSG over a TLS connection
    Sender->>DB: Mark sent after a successful write
    Receiver->>Receiver: Verify author signature
    Receiver-->>Sender: MSG_ACK with message ID
    Receiver->>Receiver: Insert unless message ID already exists
    Sender->>DB: Mark delivered when ACK arrives
```

Messages use UUIDv7 IDs when generation succeeds. The sender waits up to three seconds for an acknowledgment. If delivery fails or the acknowledgment does not arrive, it adds the message to the persistent outbox. A worker checks due entries every two seconds, and discovery events can trigger retries earlier. Outbox entries have a seven-day expiry.

An ACK currently precedes the receiver's database insert. The displayed `delivered` status therefore confirms an acknowledgment, not a guarantee that the receiver committed the message to disk. Room ACKs are matched by message ID and recipient. The displayed status is shared by the room message, so it does not indicate that every member has acknowledged it.

### Finding rooms

The application searches for available rooms every three seconds, including in REPL mode. Each search sends a wildcard `ROOM_QUERY` over UDP to broadcast targets and known peer addresses. Active members reply with room names and metadata; duplicate catalog queries from the same peer are rate limited. Searching does not join rooms, open chat connections, or synchronize message history.

Available rooms appear separately from joined rooms in the sidebar. `/room list` shows both groups, and Tab completes discovered names after `/join`. The TUI reports changes in the system panel so names remain discoverable on narrow terminals. Unchanged scans do not add log entries. A room disappears from the available list when no host has answered for ten seconds; joined rooms remain in local history.

The traffic is small metadata packets, but grows with the number of peers and rooms. This is intended for a small office LAN. Both hosts must run a version that supports wildcard room searches; ordinary network broadcast restrictions still apply.

### Room lifecycle

Room names are normalized to lowercase and must contain 2-32 letters, digits, hyphens, or underscores. A room record includes a name, generation timestamp, master ID, local role, and state.

`/room new <name>` creates a local room and adds the creator as its master. `/join <name>` sends `ROOM_QUERY` requests, even when a local room already exists. The reply goes to the query's UDP source address, rather than its advertised TCP port. A received `ROOM_HERE` response supplies metadata for joining. If no response arrives, the application reopens an active local room or creates a new room when none exists. A blocked discovery path can still cause separate rooms with the same name.

After joining, the process opens a chat connection and sends a signed join event before requesting history. The receiving member checks the room generation, adds the joining peer, and distributes `ROOM_META` to the recorded members. Receivers merge the member list for their active room. Room messages are then sent directly to recorded members; room control and history requests can establish a connection from discovery or saved peer addresses when necessary. The master controls ownership transfer and disbanding; it is not a chat relay.

History synchronization requests unknown messages from the last seven days, within the serving peer's latest 500 stored messages. Responses are split to fit the protocol frame limit. Each message must match the requested room, its author's key fingerprint, and its signature before insertion. Known message IDs prevent duplicates. Join succeeds after the responding peer confirms membership; a missing confirmation produces an error instead of reporting a completed join. The TUI reloads the active room when history arrives, preserving the current draft. History can only be recovered from peers that still store it.

Leaving marks local membership as left. Disbanding records a tombstone for that room generation and sends a signed close event. The current implementation does not provide reliable propagation of every room event to offline members.

### Direct file transfer

The TUI picker asks for confirmation before handing the selected file to the transfer manager. `/send` skips that confirmation. Both entry points resolve the path and check the file type and configured size limit before hashing.

1. The sender hashes the file with SHA-256. The hash becomes its `file_id`.
2. It opens a separate TLS connection and sends `FILE_OFFER` with the filename, size, and chunk count.
3. The receiver creates or reuses a `.part` file and a `.meta` bitmap. A set bit means that chunk is still needed.
4. The receiver returns the bitmap in `FILE_ACCEPT`. The sender skips chunks already marked complete.
5. Each incoming chunk is written at its indexed offset, then its bitmap bit is cleared on disk.
6. Once all chunks are present, the receiver hashes the complete file. A matching hash leads to the final filename and `FILE_DONE` response.
7. The sender reports success only after receiving an `ok` completion response.

The persisted bitmap supports resuming when the same file is offered again. Automatic redial after disconnection is not implemented. Incoming offers are checked for the configured size limit, hash format, chunk layout, and safe filename. Direct offers are currently accepted without a consent prompt.

### Room file downloads

A room upload creates a signed attachment message in chat history, is cached by SHA-256, and is announced to current members with `FILE_ANNOUNCE`. Each receiver assigns a local number; repeated announcements for the same room, sender, and file retain that number. Announcement numbers are kept in memory for the current process. Full file IDs can also be resolved from signed attachment history, including after restarting or joining later, while the source still has a cached copy.

`/get <number>` and `/get <full-file-ID>` open a separate file connection and sends `FILE_REQUEST`. The owner checks room membership and the cached file's hash, then sends `FILE_OFFER` back over that connection. The receiver's bitmap selects missing chunks. The receiver reports completion only after hash verification, a successful rename into the downloads folder, and an updated transfer record.

If the owner cannot serve the request, the requester sends `FILE_WHO_HAS` to other recorded room members. A member with a cached copy replies with `FILE_HAVE` and can serve the same request. Completed downloads remain available as cache sources for seven days. This fallback selects one source at a time; it does not download chunks from several peers simultaneously. A missing source or failed save is reported as an error.

DM attachments normally download automatically. `/get <full-file-ID>` displays an existing verified download or requests the file again from the original sender. A sender can also save its own cached attachment by ID. Knowing a DM file ID does not grant unrelated peers permission to retrieve it.

Use the file ID entry in Ctrl+Y, not a user ID. Both are hashes, but `/get` only resolves known file attachments. For example:

```text
/get 3
/get <paste-the-complete-file-ID-here>
```

### UI events

The TUI runs input handling and model updates in Bubble Tea's event loop. Operations that can trigger service callbacks run as commands outside that loop. Their callbacks send messages back to the program, and `Update` changes the displayed state.

This ordering matters: a synchronous service callback that calls `Program.Send` from inside `Update` can block the same loop needed to receive it. Room creation and message sending use asynchronous commands to avoid that deadlock.

The file picker keeps its own selection and recipient while leaving the chat draft intact. Directory reads return messages tagged with a generation number, so results from a canceled picker cannot overwrite a newly opened picker. While the picker is open, its keyboard handler receives input before the chat handler; Esc cancels selection instead of minimizing chat.

## Local storage

The application uses `os.UserConfigDir` and `os.UserCacheDir` for its default locations.

| Platform | Configuration and database | Cache root |
|---|---|---|
| Linux | `~/.config/kerykeion/` | `~/.cache/kerykeion/` |
| Windows | `%APPDATA%\kerykeion\` | `%LOCALAPPDATA%\kerykeion\` |
| macOS | `~/Library/Application Support/kerykeion/` | `~/Library/Caches/kerykeion/` |

The configuration directory contains `identity.key`, `config.json`, and `history.db`. Completed files go to the operating system Downloads directory: Linux follows `XDG_DOWNLOAD_DIR` from `user-dirs.dirs` (falling back to `~/Downloads`), macOS uses `~/Downloads`, and Windows uses the Downloads Known Folder, including redirected locations. The completion message displays the exact saved path. Partial files and upload caches remain under the application cache root.

Older configurations pointing to the application cache's `downloads/` directory automatically use the new default. Existing files are left where they were. An explicitly configured alternative `downloads_dir` is preserved. `--config <dir>` still isolates downloads under `<dir>/downloads` for testing. Uninstalling does not delete completed files from the OS Downloads folder.

SQLite runs in WAL mode. It stores peers, rooms, membership, tombstones, messages, outbox entries, transfer records, and media cache metadata. File contents are stored on disk rather than inside SQLite.

Startup cleanup removes messages older than the configured retention period, expired outbox entries, expired tombstones, and expired media cache records. The default retention period is 30 days. Removing a cache record does not currently delete its file. Partial files older than 24 hours are removed during transfer-manager startup.

## Current limitations

- Local announcement numbers are not persisted. Use the full file ID from attachment history after restarting. Multi-source parallel downloads and automatic transfer redial are not implemented.
- `HELLO` IDs must match an existing TLS-derived peer ID, but incoming TLS still allows clients without a certificate. Full inbound identity enforcement needs work.
- Incoming direct file offers are accepted without a consent prompt. Room file requests are restricted to recorded members.
- `/add` sends a UDP announcement; it does not establish a direct TCP connection. It currently advertises the supplied port rather than the caller's actual listening port.
- `/doctor` checks local interfaces and socket behavior. Its UDP test is loopback, not a test of broadcast delivery between laptops.
- Two local instances need separate configuration directories. Different UDP ports prevent ordinary discovery between them; adding IP aliases alone does not isolate listeners bound to all addresses.

These are implementation gaps, not network configuration instructions. Opening firewall ports does not fix them.

## Tests

```bash
# Run all tests
go test -v ./...

# Check for races in the exercised code paths
go test -race ./...

# Run the two-peer integration test on one machine
go test -v ./cmd/kerxt -run TestE2EMessagingAndFileTransfer -count=1

# Exercise room joining, history, chat, and file downloads over local UDP/TLS
go test -race -v ./cmd/kerxt -run TestE2ERoom -count=1
```

The integration test runs two identities and databases with local TLS connections. It checks a direct message, its ACK, local room creation, and a small direct file transfer. Peer addresses are seeded directly; the test does not exercise UDP discovery.

Room integration tests use three independent identities, databases, TCP listeners, and UDP sockets. Signed discovery announcements are sent to explicit local UDP addresses; room queries, metadata, history, and messages then use the production callbacks and dispatcher. They cover joining after a separate local room was created, three-member chat, history larger than one frame, and repeated joins without duplicate messages. Room download tests exercise live announcements, empty and multi-chunk files, cache fallback, local numbering, filename collisions, partial-file resume, unavailable files, and failed saves. These tests do not exercise broadcast routing between physical devices.

UI tests cover history refresh, room creation with a running event loop, input routing by panel focus, language persistence, minimize/restore behavior, terminal sizing, and scrollbar movement. These tests do not establish real-device behavior across access points, interrupted transfers, or platform-specific firewall settings.

## License

Kerykeion Terminal uses the [Kerykeion Personal, Educational, and Internal Use License 1.0](LICENSE). This is a custom source-available license, not an OSI-approved open-source license.

| Use | Permission |
|---|---|
| Personal, noncommercial use and modification | Allowed |
| Noncommercial teaching, learning, and research | Allowed |
| Internal chat and file transfers at an office or business | Allowed, including for-profit organizations |
| Free redistribution of original or modified copies | Allowed with the license, existing notices, and identification of changes |
| Selling the source code, binary, or a modified version | Not allowed |
| Paid hosted access, subscriptions, or inclusion in a paid software product | Not allowed |
| Providing the application to customers as a business product or service | Not covered by the internal-use permission |

Separate written permission is required for uses outside these terms. Third-party dependencies retain their own licenses. The full [LICENSE](LICENSE) governs; this table is a summary.
