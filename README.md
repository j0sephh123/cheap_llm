# ctx - Context Accumulator TUI

A terminal UI tool for accumulating project context, requests, and files into a single clipboard output for pasting into LLM chats.

## Prerequisites

- **Go 1.24+** (see [golang.org/dl](https://golang.org/dl/))
- **Linux**: `xclip` or `xsel` for clipboard support
  ```bash
  # Debian/Ubuntu
  sudo apt install xclip
  # or
  sudo apt install xsel
  ```
- **macOS**: Clipboard works out of the box via `pbcopy`

## Building

Clone the repository and build:

```bash
git clone https://github.com/yourusername/ctx.git
cd ctx
go build -o ctx .
```

## Installation

After building, move the binary to a directory in your PATH:

```bash
# Option 1: Install to /usr/local/bin (requires sudo)
sudo mv ctx /usr/local/bin/

# Option 2: Install to ~/bin (user-local)
mkdir -p ~/bin
mv ctx ~/bin/
# Make sure ~/bin is in your PATH
```

Or install directly with Go:

```bash
go install
```

## Running

```bash
./ctx
```

Or if installed to PATH:

```bash
ctx
```

## Configuration

Config files are stored in `~/.ctx/`:

```
~/.ctx/
├── config.yaml       # active context and exclude rule
├── contexts/         # saved contexts
├── excludes/         # exclude patterns
└── history/          # yanked prompt history
```

## License

MIT
