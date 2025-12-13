# ctx - Context Accumulator TUI

A Bubble Tea TUI tool for accumulating project context, requests, and files into a single clipboard output, designed for pasting into LLM chats.

## Usage

```bash
./ctx
```

## UI Layout

**Top bar**: Tab switcher (`[Context]` / `[History]`) with `<`/`>` navigation

### Context Tab (default)
Split view with:
- **Left side**: Three bordered boxes (Request, Files, Project Context)
- **Right side**: Preview of the output that will be yanked
- **Top**: Context names showing all available contexts
- **Bottom**: Keybindings help

Active box is highlighted with cyan border and ▸ marker.

### History Tab
Split view with:
- **Left side**: List of previously yanked prompts (timestamp, context name)
- **Right side**: Preview of selected entry (project context, request, files)
- Navigate with `↑/↓` or `j/k`
- Press `y` to yank selected entry to clipboard

## Keybindings

### Main View
| Key | Action |
|-----|--------|
| `<` / `>` | Switch between Context and History tabs |
| `y` | Yank to clipboard (also saves to history) |
| `d` | Delete selected/cursor file |
| `D` | Clear all files |
| `*` | Select/deselect all |
| `a` | Add file/directory |
| `f` | Toggle folder view |
| `e` / `Enter` | Edit active box (Request or Project Context) |
| `Tab` / `Shift+Tab` | Switch between boxes |
| `{` / `}` | Switch between contexts |
| `c` | Open context selection menu |
| `E` | Switch exclude rule |
| `r` | Reload from disk |
| `s` | Show current config |
| `Space` | Toggle file selection |
| `↑/↓` or `j/k` | Navigate files (or history entries) |
| `q` | Quit |

### Context Selection (`c`)
| Key | Action |
|-----|--------|
| `Enter` | Select context |
| `D` | Delete context (not allowed for "default") |
| `Esc` | Cancel |

### Edit Mode (`e`)
| Key | Action |
|-----|--------|
| `Enter` | Save and exit |
| `Esc` | Cancel without saving |

### Folder View (`f`)
| Key | Action |
|-----|--------|
| `d` | Delete files in selected folders |
| `Space` | Toggle folder selection |
| `f` / `Esc` | Back to file view |

## Config Structure

```
~/.ctx/
├── config.yaml              # active_context, active_exclude, skip_prefixes
├── contexts/
│   └── default.yaml         # name, project_root, project_context, request, files[]
├── excludes/
│   └── default.yaml         # name, patterns[]
└── history/
    └── 2025-01-15_14-30-45_default.yaml  # timestamp_contextname.yaml
```

## Context YAML Format

```yaml
name: my-project
project_root: /home/user/projects/my-project  # optional: makes file paths relative
project_context: |
  Go CLI tool using Bubble Tea for TUI.
  Config stored in ~/.ctx/
request: |
  Add a new feature to handle user authentication.
files:
  - /home/user/projects/my-project/main.go
  - /home/user/projects/my-project/config.go
```

### project_root

When `project_root` is set, file paths in the yanked output become relative:
- Without: `<file path="/home/user/projects/my-project/main.go">`
- With: `<file path="main.go">`

## History Entry YAML Format

History entries are saved automatically when you yank (`y`). Each entry stores metadata only (no file contents):

```yaml
timestamp: 2025-01-15T14:30:45Z
context_name: my-project
project_context: |
  Go CLI tool using Bubble Tea for TUI.
request: |
  Add a new feature to handle user authentication.
files:
  - /home/user/projects/my-project/main.go
  - /home/user/projects/my-project/config.go
```

- Maximum 100 entries are kept (oldest are auto-deleted)
- Filename format: `YYYY-MM-DD_HH-MM-SS_contextname.yaml`

## Output Format (yanked to clipboard)

The output includes a preamble explaining the structure to the LLM:

```
This is a structured prompt for a software development task.

<project_context> describes the project: its purpose, tech stack, architecture, and coding conventions.

<request> contains the specific task or question to address.

<file> tags contain the relevant source files.

---

<project_context>
...what the project is about...
</project_context>

<request>
...what you want to achieve...
</request>

<file path="main.go">
...file contents...
</file>
```

## Default Excludes

The default exclude rule filters out:
- `**/node_modules/**`
- `**/.git/**`
- `**/.env`, `**/.env.*`, `**/*.env`
- `**/package-lock.json`
- `**/pnpm-lock.yaml`
- `**/yarn.lock`

## Tech Stack

- Go + Bubble Tea + Lipgloss
- Clipboard: atotto/clipboard with xclip/xsel fallback
- Glob matching: bmatcuk/doublestar
- Text editing: charmbracelet/bubbles/textarea
