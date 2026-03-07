# ccs — Claude Code Sessions

A TUI for managing [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions. Easily browse sessions and resume them.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)

## What it does

- Lists all your Claude Code sessions with project, title, context usage, and age
- Resume any session with a keypress or launch a new one in a project directory
- Fuzzy search, sorting, session hiding, and active session detection

## Install

```bash
go install github.com/myaiexp/ccs@latest
```

Or build from source:

```bash
git clone https://github.com/myaiexp/ccs.git
cd ccs
go build -o ~/.local/bin/ccs .
```

## Usage

```bash
ccs
```

Press `?` for keybindings.

## License

MIT
