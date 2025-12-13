package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// UI modes
type mode int

const (
	modeNormal mode = iota
	modeFolderView
	modeContextSelect
	modeExcludeSelect
	modeNewContext
	modeAddFile
	modeShowConfig
	modeEditBox          // editing Request or Project Context
	modeConfirmDeleteCtx // confirming context deletion
)

// Tab constants for main view
type mainTab int

const (
	tabContext mainTab = iota
	tabHistory
)

// FileInfo holds display information for a file
type FileInfo struct {
	Path     string
	Project  string
	RelPath  string
	Size     int64
	Exists   bool
	Selected bool
}

// FolderInfo holds aggregated info for a folder
type FolderInfo struct {
	Path      string
	FileCount int
	TotalSize int64
	Selected  bool
}

// Active box constants (order matches visual layout: Request, Files, Project Context)
const (
	boxRequest = iota
	boxFiles
	boxProjectContext
)

// Model is the Bubble Tea model
type Model struct {
	config      Config
	context     Context
	contexts    []string // list of all context names
	exclude     ExcludeRule
	files       []FileInfo
	folders     []FolderInfo
	cursor      int
	offset      int // scroll offset
	folderCursor int
	folderOffset int
	mode        mode
	inputBuffer string
	activeBox   int // 0=request, 1=files, 2=project_context

	// For context/exclude selection
	selectItems  []string
	selectCursor int

	// For editing text boxes
	textArea    textarea.Model
	editingBox  int // which box is being edited (-1 = none)

	// For delete confirmation
	deleteTarget string // context name to delete

	// Main view tab (context or history)
	activeTab      mainTab
	historyEntries []HistoryEntry
	historyCursor  int
	historyOffset  int

	// Terminal size
	width  int
	height int
}

func initialModel() Model {
	m := Model{
		mode:       modeNormal,
		width:      80,
		height:     24,
		editingBox: -1,
	}

	// Ensure config directory exists
	if err := EnsureConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config dir: %v\n", err)
		os.Exit(1)
	}

	// Load config
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	m.config = cfg

	// Load active context (fall back to "default" if not found)
	ctx, err := LoadContext(cfg.ActiveContext)
	if err != nil {
		// Try loading default context instead
		ctx, err = LoadContext("default")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading context: %v\n", err)
			os.Exit(1)
		}
		// Update config to use default
		cfg.ActiveContext = "default"
		SaveConfig(cfg)
	}
	m.context = ctx

	// Load active exclude rule
	exc, err := LoadExcludeRule(cfg.ActiveExclude)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading exclude: %v\n", err)
		os.Exit(1)
	}
	m.exclude = exc

	// Load list of all contexts
	contexts, err := ListContexts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing contexts: %v\n", err)
		os.Exit(1)
	}
	m.contexts = contexts

	// Build file info list
	m.refreshFiles()

	return m
}

func (m *Model) refreshFiles() {
	m.files = make([]FileInfo, len(m.context.Files))
	for i, path := range m.context.Files {
		m.files[i] = m.buildFileInfo(path)
	}

	// Sort by size descending (largest first)
	sort.Slice(m.files, func(i, j int) bool {
		return m.files[i].Size > m.files[j].Size
	})

	m.refreshFolders()
}

func (m *Model) refreshFolders() {
	// Group files by parent directory
	folderMap := make(map[string]*FolderInfo)

	for _, f := range m.files {
		dir := filepath.Dir(f.Path)
		if folder, exists := folderMap[dir]; exists {
			folder.FileCount++
			folder.TotalSize += f.Size
		} else {
			folderMap[dir] = &FolderInfo{
				Path:      dir,
				FileCount: 1,
				TotalSize: f.Size,
			}
		}
	}

	// Convert map to slice
	m.folders = make([]FolderInfo, 0, len(folderMap))
	for _, folder := range folderMap {
		m.folders = append(m.folders, *folder)
	}

	// Sort by path
	sort.Slice(m.folders, func(i, j int) bool {
		return m.folders[i].Path < m.folders[j].Path
	})
}

func (m *Model) buildFileInfo(path string) FileInfo {
	info := FileInfo{
		Path:   path,
		Exists: true,
	}

	// Check if file exists and get size
	stat, err := os.Stat(path)
	if err != nil {
		info.Exists = false
		info.Size = 0
	} else {
		info.Size = stat.Size()
	}

	// Build display path
	home, _ := os.UserHomeDir()
	relPath := path
	if strings.HasPrefix(path, home) {
		relPath = strings.TrimPrefix(path, home+"/")
	}

	// Extract project name
	parts := strings.Split(relPath, "/")
	projectIdx := 0

	// Skip known prefixes
	for i, part := range parts {
		skip := false
		for _, prefix := range m.config.SkipPrefixes {
			if part == prefix {
				skip = true
				break
			}
		}
		if !skip {
			projectIdx = i
			break
		}
	}

	if projectIdx < len(parts) {
		info.Project = parts[projectIdx]
		if projectIdx+1 < len(parts) {
			info.RelPath = strings.Join(parts[projectIdx+1:], "/")
		} else {
			info.RelPath = ""
		}
	} else {
		info.Project = ""
		info.RelPath = relPath
	}

	return info
}

func (m *Model) totalSize() int64 {
	var total int64
	for _, f := range m.files {
		total += f.Size
	}
	return total
}

func (m *Model) selectedCount() int {
	count := 0
	for _, f := range m.files {
		if f.Selected {
			count++
		}
	}
	return count
}

func (m *Model) setStatus(msg string) tea.Cmd {
	return nil
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return tea.EnableBracketedPaste()
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Check if this is a paste event
		if msg.Paste {
			pastedText := string(msg.Runes)
			if m.mode == modeNormal {
				return m, m.processPaste(pastedText)
			} else if m.mode == modeAddFile {
				m.inputBuffer += pastedText
				return m, nil
			}
		}
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeNormal:
		return m.handleNormalKey(msg)
	case modeFolderView:
		return m.handleFolderKey(msg)
	case modeContextSelect:
		return m.handleSelectKey(msg, "context")
	case modeExcludeSelect:
		return m.handleSelectKey(msg, "exclude")
	case modeNewContext:
		return m.handleNewContextKey(msg)
	case modeAddFile:
		return m.handleAddFileKey(msg)
	case modeShowConfig:
		return m.handleShowConfigKey(msg)
	case modeEditBox:
		return m.handleEditBoxKey(msg)
	case modeConfirmDeleteCtx:
		return m.handleConfirmDeleteKey(msg)
	}
	return m, nil
}

func (m Model) handleConfirmDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "y", "Y":
		// Confirm deletion
		if err := DeleteContext(m.deleteTarget); err != nil {
			m.mode = modeNormal
			return m, m.setStatus(fmt.Sprintf("Error deleting: %v", err))
		}

		// If we deleted the active context, switch to another one
		if m.deleteTarget == m.context.Name {
			contexts, _ := ListContexts()
			if len(contexts) > 0 {
				m.switchToContext(contexts[0])
			}
		}

		// Refresh contexts list
		contexts, _ := ListContexts()
		m.contexts = contexts

		m.mode = modeNormal
		m.deleteTarget = ""
		return m, m.setStatus("Context deleted")

	case "n", "N", "esc", "q":
		// Cancel
		m.mode = modeContextSelect
		m.deleteTarget = ""
		return m, nil
	}

	return m, nil
}

func (m Model) handleEditBoxKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Save and exit edit mode
		if m.editingBox == boxRequest {
			m.context.Request = m.textArea.Value()
		} else if m.editingBox == boxProjectContext {
			m.context.ProjectContext = m.textArea.Value()
		}
		SaveContext(m.context)
		m.mode = modeNormal
		m.editingBox = -1
		return m, nil

	case tea.KeyEsc, tea.KeyCtrlC:
		// Cancel without saving
		m.mode = modeNormal
		m.editingBox = -1
		return m, nil
	}

	// Pass other keys to textarea
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return m, cmd
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	visibleRows := m.visibleFileRows()

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.activeTab == tabHistory {
			// Navigate history
			if m.historyCursor > 0 {
				m.historyCursor--
				if m.historyCursor < m.historyOffset {
					m.historyOffset = m.historyCursor
				}
			}
		} else {
			// Navigate files
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		}

	case "down", "j":
		if m.activeTab == tabHistory {
			// Navigate history
			if m.historyCursor < len(m.historyEntries)-1 {
				m.historyCursor++
				if m.historyCursor >= m.historyOffset+visibleRows {
					m.historyOffset = m.historyCursor - visibleRows + 1
				}
			}
		} else {
			// Navigate files
			if m.cursor < len(m.files)-1 {
				m.cursor++
				if m.cursor >= m.offset+visibleRows {
					m.offset = m.cursor - visibleRows + 1
				}
			}
		}

	case " ":
		// Toggle selection
		if m.cursor < len(m.files) {
			m.files[m.cursor].Selected = !m.files[m.cursor].Selected
		}

	case "*":
		// Select/deselect all
		allSelected := true
		for _, f := range m.files {
			if !f.Selected {
				allSelected = false
				break
			}
		}
		for i := range m.files {
			m.files[i].Selected = !allSelected
		}

	case "D":
		// Clear all files
		m.context.Files = []string{}
		SaveContext(m.context)
		m.refreshFiles()
		m.cursor = 0
		m.offset = 0

	case "y":
		if m.activeTab == tabHistory {
			return m, m.yankHistoryEntry()
		}
		return m, m.yank()

	case "d":
		return m, m.deleteSelected()

	case "c":
		return m.enterContextSelect()

	case "E":
		return m.enterExcludeSelect()

	case "r":
		return m.reload()

	case "s":
		m.mode = modeShowConfig
		return m, nil

	case "a":
		m.mode = modeAddFile
		m.inputBuffer = ""
		return m, nil

	case "f":
		m.mode = modeFolderView
		m.folderCursor = 0
		m.folderOffset = 0
		return m, nil

	case "[", "shift+tab":
		// Previous box
		m.activeBox--
		if m.activeBox < 0 {
			m.activeBox = boxProjectContext
		}

	case "]", "tab":
		// Next box
		m.activeBox++
		if m.activeBox > boxProjectContext {
			m.activeBox = boxRequest
		}

	case "{":
		// Previous context
		if len(m.contexts) > 1 {
			currentIdx := -1
			for i, name := range m.contexts {
				if name == m.context.Name {
					currentIdx = i
					break
				}
			}
			if currentIdx > 0 {
				m.switchToContext(m.contexts[currentIdx-1])
			} else {
				m.switchToContext(m.contexts[len(m.contexts)-1])
			}
		}

	case "}":
		// Next context
		if len(m.contexts) > 1 {
			currentIdx := -1
			for i, name := range m.contexts {
				if name == m.context.Name {
					currentIdx = i
					break
				}
			}
			if currentIdx < len(m.contexts)-1 {
				m.switchToContext(m.contexts[currentIdx+1])
			} else {
				m.switchToContext(m.contexts[0])
			}
		}

	case "enter", "e":
		// Enter edit mode for Request or Project Context (only in context tab)
		if m.activeTab == tabContext && (m.activeBox == boxRequest || m.activeBox == boxProjectContext) {
			return m.enterEditMode()
		}

	case "<":
		// Switch to previous tab
		if m.activeTab == tabHistory {
			m.activeTab = tabContext
		}

	case ">":
		// Switch to next tab (history)
		if m.activeTab == tabContext {
			m.activeTab = tabHistory
			// Load history entries when switching to history tab
			entries, _ := ListHistoryEntries()
			m.historyEntries = entries
			m.historyCursor = 0
			m.historyOffset = 0
		}
	}

	return m, nil
}

func (m Model) enterEditMode() (tea.Model, tea.Cmd) {
	// Create textarea with current content
	ta := textarea.New()
	ta.Placeholder = "Type here..."
	ta.ShowLineNumbers = false
	ta.SetWidth(m.width/2 - 6)
	ta.SetHeight(m.height/3 - 2)

	if m.activeBox == boxRequest {
		ta.SetValue(m.context.Request)
	} else {
		ta.SetValue(m.context.ProjectContext)
	}

	ta.Focus()
	m.textArea = ta
	m.editingBox = m.activeBox
	m.mode = modeEditBox

	return m, textarea.Blink
}

// visibleFileRows returns how many file rows can be displayed
func (m Model) visibleFileRows() int {
	// Reserve lines for: title, separator, files header, separator, keybindings
	reserved := 5
	available := m.height - reserved
	if available < 3 {
		available = 3
	}
	return available
}

func (m *Model) switchToContext(name string) {
	ctx, err := LoadContext(name)
	if err != nil {
		return
	}
	m.context = ctx
	m.config.ActiveContext = name
	SaveConfig(m.config)
	m.refreshFiles()
	m.cursor = 0
	m.offset = 0
}

func (m Model) handleFolderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	visibleRows := m.visibleFileRows()

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "f", "esc":
		// Back to file view
		m.mode = modeNormal
		return m, nil

	case "up", "k":
		if m.folderCursor > 0 {
			m.folderCursor--
			if m.folderCursor < m.folderOffset {
				m.folderOffset = m.folderCursor
			}
		}

	case "down", "j":
		if m.folderCursor < len(m.folders)-1 {
			m.folderCursor++
			if m.folderCursor >= m.folderOffset+visibleRows {
				m.folderOffset = m.folderCursor - visibleRows + 1
			}
		}

	case " ":
		// Toggle selection
		if m.folderCursor < len(m.folders) {
			m.folders[m.folderCursor].Selected = !m.folders[m.folderCursor].Selected
		}

	case "d":
		// Delete files in selected folders (or cursor folder)
		var foldersToDelete []string
		hasSelection := false
		for _, folder := range m.folders {
			if folder.Selected {
				hasSelection = true
				foldersToDelete = append(foldersToDelete, folder.Path)
			}
		}
		if !hasSelection && m.folderCursor < len(m.folders) {
			foldersToDelete = []string{m.folders[m.folderCursor].Path}
		}

		// Remove files that are in these folders
		var newFiles []string
		for _, file := range m.context.Files {
			dir := filepath.Dir(file)
			keep := true
			for _, folder := range foldersToDelete {
				if dir == folder {
					keep = false
					break
				}
			}
			if keep {
				newFiles = append(newFiles, file)
			}
		}
		m.context.Files = newFiles
		SaveContext(m.context)
		m.refreshFiles()

		// Adjust cursor
		if m.folderCursor >= len(m.folders) && m.folderCursor > 0 {
			m.folderCursor = len(m.folders) - 1
		}

		// If no folders left, go back to normal view
		if len(m.folders) == 0 {
			m.mode = modeNormal
		}
	}

	return m, nil
}

func (m Model) handleSelectKey(msg tea.KeyMsg, selectType string) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "q", "ctrl+c", "esc":
		m.mode = modeNormal
		return m, nil

	case "up", "k":
		if m.selectCursor > 0 {
			m.selectCursor--
		}

	case "down", "j":
		if m.selectCursor < len(m.selectItems)-1 {
			m.selectCursor++
		}

	case "D":
		// Delete context (only for context select, not exclude)
		if selectType == "context" && m.selectCursor < len(m.selectItems) {
			selected := m.selectItems[m.selectCursor]
			// Don't allow deleting "[+] New context" or "default"
			if selected != "[+] New context" && selected != "default" {
				m.deleteTarget = selected
				m.mode = modeConfirmDeleteCtx
				return m, nil
			}
		}

	case "enter":
		if m.selectCursor < len(m.selectItems) {
			selected := m.selectItems[m.selectCursor]

			if selectType == "context" {
				if selected == "[+] New context" {
					m.mode = modeNewContext
					m.inputBuffer = ""
					return m, nil
				}
				// Switch context
				ctx, err := LoadContext(selected)
				if err != nil {
					m.mode = modeNormal
					return m, m.setStatus(fmt.Sprintf("Error: %v", err))
				}
				m.context = ctx
				m.config.ActiveContext = selected
				SaveConfig(m.config)
				m.refreshFiles()
				m.cursor = 0
			} else {
				// Switch exclude
				exc, err := LoadExcludeRule(selected)
				if err != nil {
					m.mode = modeNormal
					return m, m.setStatus(fmt.Sprintf("Error: %v", err))
				}
				m.exclude = exc
				m.config.ActiveExclude = selected
				SaveConfig(m.config)
			}
		}
		m.mode = modeNormal
		return m, nil
	}

	return m, nil
}

func (m Model) handleNewContextKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		return m, nil

	case tea.KeyEnter:
		if m.inputBuffer != "" {
			// Create new context
			ctx := Context{
				Name:           m.inputBuffer,
				ProjectContext: "",
				Request:        "",
				Files:          []string{},
			}
			if err := SaveContext(ctx); err != nil {
				m.mode = modeNormal
				return m, m.setStatus(fmt.Sprintf("Error: %v", err))
			}
			// Switch to it
			m.context = ctx
			m.config.ActiveContext = m.inputBuffer
			SaveConfig(m.config)
			m.refreshFiles()
			m.cursor = 0
			m.mode = modeNormal
			return m, m.setStatus(fmt.Sprintf("Created context: %s", m.inputBuffer))
		}
		m.mode = modeNormal
		return m, nil

	case tea.KeyBackspace:
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}

	case tea.KeyRunes:
		m.inputBuffer += string(msg.Runes)
	}

	return m, nil
}

func (m Model) handleAddFileKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		return m, nil

	case tea.KeyEnter:
		if m.inputBuffer != "" {
			cmd := m.processPaste(m.inputBuffer)
			m.inputBuffer = ""
			m.mode = modeNormal
			return m, cmd
		}
		m.mode = modeNormal
		return m, nil

	case tea.KeyBackspace:
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}

	case tea.KeyRunes:
		// This handles both single chars and pasted text
		m.inputBuffer += string(msg.Runes)
	}

	return m, nil
}

func (m Model) handleShowConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	return m, nil
}

func (m *Model) processPaste(input string) tea.Cmd {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Check if it's an absolute path
	if !strings.HasPrefix(input, "/") {
		return m.setStatus("Not a valid path")
	}

	// Check if path exists
	stat, err := os.Stat(input)
	if err != nil {
		return m.setStatus(fmt.Sprintf("Path not found: %s", input))
	}

	if stat.IsDir() {
		// Expand directory
		files, err := ExpandDirectory(input, &m.exclude)
		if err != nil {
			return m.setStatus(fmt.Sprintf("Error expanding: %v", err))
		}

		added := 0
		for _, f := range files {
			if m.context.AddFile(f) {
				added++
			}
		}

		if err := SaveContext(m.context); err != nil {
			return m.setStatus(fmt.Sprintf("Error saving: %v", err))
		}

		m.refreshFiles()
		return m.setStatus(fmt.Sprintf("Added %d files from directory", added))
	}

	// Single file
	if m.context.AddFile(input) {
		if err := SaveContext(m.context); err != nil {
			return m.setStatus(fmt.Sprintf("Error saving: %v", err))
		}
		m.refreshFiles()
		return m.setStatus("File added")
	}

	return m.setStatus("Already in context")
}

func (m *Model) yank() tea.Cmd {
	var sb strings.Builder

	// Write preamble explaining the structure
	sb.WriteString(`This is a structured prompt for a software development task.

<project_context> describes the project: its purpose, tech stack, architecture, and coding conventions. Use this to understand the broader context.

<request> contains the specific task or question to address. This is what you should focus on accomplishing.

<file> tags contain the relevant source files. Each file has a path attribute. Use these to understand the current implementation and make appropriate changes.

---

`)

	// Write project context
	if m.context.ProjectContext != "" {
		sb.WriteString("<project_context>\n")
		sb.WriteString(m.context.ProjectContext)
		if !strings.HasSuffix(m.context.ProjectContext, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</project_context>\n\n")
	}

	// Write request
	if m.context.Request != "" {
		sb.WriteString("<request>\n")
		sb.WriteString(m.context.Request)
		if !strings.HasSuffix(m.context.Request, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</request>\n\n")
	}

	// Check for missing files
	var missing []string
	for _, f := range m.files {
		if !f.Exists {
			missing = append(missing, f.Path)
		}
	}

	if len(missing) > 0 {
		return m.setStatus(fmt.Sprintf("Warning: %d file(s) missing", len(missing)))
	}

	// Write files
	for _, f := range m.files {
		if !f.Exists {
			continue
		}

		content, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}

		// Use relative path if project_root is set
		displayPath := f.Path
		if m.context.ProjectRoot != "" {
			root := m.context.ProjectRoot
			if !strings.HasSuffix(root, "/") {
				root += "/"
			}
			if strings.HasPrefix(f.Path, root) {
				displayPath = strings.TrimPrefix(f.Path, root)
			}
		}

		sb.WriteString(fmt.Sprintf("<file path=\"%s\">\n", displayPath))
		sb.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			sb.WriteString("\n")
		}
		sb.WriteString("</file>\n\n")
	}

	// Copy to clipboard
	if err := CopyToClipboard(sb.String()); err != nil {
		return m.setStatus(fmt.Sprintf("Clipboard error: %v", err))
	}

	// Save to history
	var filePaths []string
	for _, f := range m.files {
		filePaths = append(filePaths, f.Path)
	}
	entry := HistoryEntry{
		Timestamp:      time.Now(),
		ContextName:    m.context.Name,
		ProjectContext: m.context.ProjectContext,
		Request:        m.context.Request,
		Files:          filePaths,
	}
	SaveHistoryEntry(entry) // Ignore error - don't fail yank if history fails

	return m.setStatus(fmt.Sprintf("Yanked %d files to clipboard", len(m.files)))
}

func (m *Model) yankHistoryEntry() tea.Cmd {
	if len(m.historyEntries) == 0 || m.historyCursor >= len(m.historyEntries) {
		return m.setStatus("No history entry selected")
	}

	entry := m.historyEntries[m.historyCursor]

	var sb strings.Builder

	// Write preamble explaining the structure
	sb.WriteString(`This is a structured prompt for a software development task.

<project_context> describes the project: its purpose, tech stack, architecture, and coding conventions. Use this to understand the broader context.

<request> contains the specific task or question to address. This is what you should focus on accomplishing.

<file> tags contain the relevant source files. Each file has a path attribute. Use these to understand the current implementation and make appropriate changes.

---

`)

	// Write project context
	if entry.ProjectContext != "" {
		sb.WriteString("<project_context>\n")
		sb.WriteString(entry.ProjectContext)
		if !strings.HasSuffix(entry.ProjectContext, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</project_context>\n\n")
	}

	// Write request
	if entry.Request != "" {
		sb.WriteString("<request>\n")
		sb.WriteString(entry.Request)
		if !strings.HasSuffix(entry.Request, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</request>\n\n")
	}

	// Write files (read from disk)
	for _, filePath := range entry.Files {
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files that can't be read
		}

		sb.WriteString(fmt.Sprintf("<file path=\"%s\">\n", filePath))
		sb.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			sb.WriteString("\n")
		}
		sb.WriteString("</file>\n\n")
	}

	// Copy to clipboard
	if err := CopyToClipboard(sb.String()); err != nil {
		return m.setStatus(fmt.Sprintf("Clipboard error: %v", err))
	}

	return m.setStatus(fmt.Sprintf("Yanked history entry (%d files)", len(entry.Files)))
}

func (m *Model) deleteSelected() tea.Cmd {
	selected := m.selectedCount()

	if selected > 0 {
		// Delete all selected
		var toRemove []string
		for _, f := range m.files {
			if f.Selected {
				toRemove = append(toRemove, f.Path)
			}
		}
		m.context.RemoveFiles(toRemove)
	} else if m.cursor < len(m.files) {
		// Delete cursor item
		m.context.RemoveFile(m.files[m.cursor].Path)
	}

	if err := SaveContext(m.context); err != nil {
		return m.setStatus(fmt.Sprintf("Error saving: %v", err))
	}

	m.refreshFiles()

	// Adjust cursor if needed
	if m.cursor >= len(m.files) && m.cursor > 0 {
		m.cursor = len(m.files) - 1
	}

	if selected > 0 {
		return m.setStatus(fmt.Sprintf("Deleted %d files", selected))
	}
	return m.setStatus("Deleted file")
}

func (m Model) enterContextSelect() (tea.Model, tea.Cmd) {
	contexts, err := ListContexts()
	if err != nil {
		return m, m.setStatus(fmt.Sprintf("Error: %v", err))
	}

	m.selectItems = append([]string{"[+] New context"}, contexts...)
	m.selectCursor = 0

	// Position cursor on current context
	for i, name := range m.selectItems {
		if name == m.config.ActiveContext {
			m.selectCursor = i
			break
		}
	}

	m.mode = modeContextSelect
	return m, nil
}

func (m Model) enterExcludeSelect() (tea.Model, tea.Cmd) {
	excludes, err := ListExcludeRules()
	if err != nil {
		return m, m.setStatus(fmt.Sprintf("Error: %v", err))
	}

	m.selectItems = excludes
	m.selectCursor = 0

	// Position cursor on current exclude
	for i, name := range m.selectItems {
		if name == m.config.ActiveExclude {
			m.selectCursor = i
			break
		}
	}

	m.mode = modeExcludeSelect
	return m, nil
}

func (m Model) reload() (tea.Model, tea.Cmd) {
	cfg, err := LoadConfig()
	if err != nil {
		return m, m.setStatus(fmt.Sprintf("Error: %v", err))
	}
	m.config = cfg

	ctx, err := LoadContext(cfg.ActiveContext)
	if err != nil {
		return m, m.setStatus(fmt.Sprintf("Error: %v", err))
	}
	m.context = ctx

	exc, err := LoadExcludeRule(cfg.ActiveExclude)
	if err != nil {
		return m, m.setStatus(fmt.Sprintf("Error: %v", err))
	}
	m.exclude = exc

	// Refresh contexts list
	contexts, err := ListContexts()
	if err == nil {
		m.contexts = contexts
	}

	m.refreshFiles()
	m.cursor = 0

	return m, m.setStatus("Reloaded")
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))
)

func (m Model) View() string {
	switch m.mode {
	case modeFolderView:
		return m.viewFolders()
	case modeContextSelect:
		return m.viewSelect("Select Context")
	case modeExcludeSelect:
		return m.viewSelect("Select Exclude Rule")
	case modeNewContext:
		return m.viewInput("New Context Name", m.inputBuffer)
	case modeAddFile:
		return m.viewInput("Add File/Directory", m.inputBuffer)
	case modeShowConfig:
		return m.viewConfig()
	case modeEditBox:
		return m.viewEditBox()
	case modeConfirmDeleteCtx:
		return m.viewConfirmDelete()
	}

	// Normal mode - split view (context or history tab)
	return m.viewSplit()
}

func (m Model) viewConfirmDelete() string {
	var sb strings.Builder

	sb.WriteString(errorStyle.Render("Delete Context"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Are you sure you want to delete '%s'?\n\n", m.deleteTarget))
	sb.WriteString(warningStyle.Render("This action cannot be undone."))
	sb.WriteString("\n\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("[y]es  [n]o"))
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) viewEditBox() string {
	var sb strings.Builder

	// Title
	title := "Edit Request"
	if m.editingBox == boxProjectContext {
		title = "Edit Project Context"
	}
	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 60)))
	sb.WriteString("\n")

	// Textarea
	sb.WriteString(m.textArea.View())
	sb.WriteString("\n")

	sb.WriteString(strings.Repeat("─", min(m.width, 60)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("[enter] save & close  [esc] cancel"))
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) viewSplit() string {
	var output strings.Builder

	// Line 1: Header with main tabs (Context / History)
	// Tab bar
	if m.activeTab == tabContext {
		output.WriteString(selectedStyle.Render("[Context]") + " ")
		output.WriteString(dimStyle.Render("[History]") + " ")
	} else {
		output.WriteString(dimStyle.Render("[Context]") + " ")
		output.WriteString(selectedStyle.Render("[History]") + " ")
	}
	output.WriteString(dimStyle.Render("</>") + "  ")

	// Context-specific info on the same line
	if m.activeTab == tabContext {
		// Show context names
		for _, name := range m.contexts {
			if name == m.context.Name {
				output.WriteString(selectedStyle.Render("(" + name + ")") + " ")
			} else {
				output.WriteString(dimStyle.Render("(" + name + ")") + " ")
			}
		}
		output.WriteString(dimStyle.Render(fmt.Sprintf("Total: %s (%d files)", formatSize(m.totalSize()), len(m.files))))
		if m.totalSize() > 600*1024 {
			output.WriteString("  " + errorStyle.Render("⚠ May exceed limits"))
		} else if m.totalSize() > 400*1024 {
			output.WriteString("  " + warningStyle.Render("⚠ Getting large"))
		}
	} else {
		output.WriteString(dimStyle.Render(fmt.Sprintf("(%d entries)", len(m.historyEntries))))
	}
	output.WriteString("\n")

	if m.activeTab == tabContext {
		// Context tab - show the normal split view
		output.WriteString(m.viewContextTab())
	} else {
		// History tab - show history list
		output.WriteString(m.viewHistoryTab())
	}

	return output.String()
}

func (m Model) viewContextTab() string {
	var output strings.Builder

	// Calculate dimensions
	halfWidth := m.width / 2
	if halfWidth < 30 {
		halfWidth = 30
	}
	leftWidth := halfWidth - 4  // account for borders
	rightWidth := halfWidth - 4

	// Box heights: total height - 2 (header + keys), divide by 3 for left boxes
	// Each box needs 2 lines for border, so content height = boxHeight - 2
	totalBoxArea := m.height - 2
	boxHeight := totalBoxArea / 3
	remainder := totalBoxArea % 3 // extra rows to distribute
	if boxHeight < 4 {
		boxHeight = 4
	}
	contentHeight := boxHeight - 2

	// Give extra rows to Files box (middle) since it usually needs more space
	filesExtraHeight := remainder

	// Create bordered boxes for left side
	requestBox := m.createBorderedBox("Request", m.context.Request, leftWidth, contentHeight, m.activeBox == boxRequest)
	filesBox := m.createBorderedFilesBox(leftWidth, contentHeight+filesExtraHeight, m.activeBox == boxFiles)
	projectBox := m.createBorderedBox("Project Context", m.context.ProjectContext, leftWidth, contentHeight, m.activeBox == boxProjectContext)

	// Create bordered preview box (spans full height)
	previewContentHeight := totalBoxArea - 2 // borders
	previewBox := m.createBorderedPreviewBox(rightWidth, previewContentHeight)

	// Split boxes into lines
	reqLines := strings.Split(requestBox, "\n")
	filesLines := strings.Split(filesBox, "\n")
	projLines := strings.Split(projectBox, "\n")
	prevLines := strings.Split(previewBox, "\n")

	// Combine left boxes vertically, then join with preview horizontally
	leftLines := append(reqLines, filesLines...)
	leftLines = append(leftLines, projLines...)

	// Render line by line
	maxLines := len(leftLines)
	if len(prevLines) > maxLines {
		maxLines = len(prevLines)
	}

	for i := 0; i < maxLines; i++ {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		output.WriteString(padRight(leftLine, halfWidth))

		if i < len(prevLines) {
			output.WriteString(prevLines[i])
		}
		output.WriteString("\n")
	}

	// Keybindings
	output.WriteString(dimStyle.Render("[y]ank [d]el [a]dd [f]olders [e]dit [r]eload [c]tx [{/}]switch [tab]box [q]uit"))

	return output.String()
}

func (m Model) viewHistoryTab() string {
	var output strings.Builder

	// Calculate dimensions (same as context tab)
	halfWidth := m.width / 2
	if halfWidth < 30 {
		halfWidth = 30
	}
	leftWidth := halfWidth - 4
	rightWidth := halfWidth - 4

	totalBoxArea := m.height - 2
	if totalBoxArea < 6 {
		totalBoxArea = 6
	}

	// Build history list box
	historyBox := m.createBorderedHistoryBox(leftWidth, totalBoxArea-2)

	// Build preview box for selected entry
	previewBox := m.createBorderedHistoryPreviewBox(rightWidth, totalBoxArea-2)

	// Split boxes into lines
	histLines := strings.Split(historyBox, "\n")
	prevLines := strings.Split(previewBox, "\n")

	// Render line by line
	maxLines := len(histLines)
	if len(prevLines) > maxLines {
		maxLines = len(prevLines)
	}

	for i := 0; i < maxLines; i++ {
		leftLine := ""
		if i < len(histLines) {
			leftLine = histLines[i]
		}
		output.WriteString(padRight(leftLine, halfWidth))

		if i < len(prevLines) {
			output.WriteString(prevLines[i])
		}
		output.WriteString("\n")
	}

	// Keybindings for history tab
	output.WriteString(dimStyle.Render("[y]ank  [↑/↓]navigate  [q]uit"))

	return output.String()
}

func (m Model) createBorderedHistoryBox(width int, height int) string {
	bc := lipgloss.Color("14") // cyan for active

	var lines []string

	if len(m.historyEntries) == 0 {
		lines = []string{dimStyle.Render("(no history yet)")}
	} else {
		visibleRows := height
		if visibleRows < 3 {
			visibleRows = 3
		}

		endIdx := m.historyOffset + visibleRows
		if endIdx > len(m.historyEntries) {
			endIdx = len(m.historyEntries)
		}

		// Show scroll indicator if there are entries above
		if m.historyOffset > 0 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("↑ %d more above", m.historyOffset)))
		}

		for i := m.historyOffset; i < endIdx; i++ {
			entry := m.historyEntries[i]
			prefix := "  "
			if i == m.historyCursor {
				prefix = "> "
			}

			// Format: timestamp | context
			timestamp := entry.FormatTimestamp()
			contextName := entry.ContextName
			maxCtxLen := width - 20
			if maxCtxLen < 8 {
				maxCtxLen = 8
			}
			if len(contextName) > maxCtxLen {
				contextName = contextName[:maxCtxLen-3] + "..."
			}

			line := fmt.Sprintf("%s%s  %s", prefix, timestamp, contextName)

			if i == m.historyCursor {
				line = cursorStyle.Render(line)
			}

			lines = append(lines, line)
		}

		// Show scroll indicator if there are entries below
		if endIdx < len(m.historyEntries) {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("↓ %d more below", len(m.historyEntries)-endIdx)))
		}
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Build box
	var box strings.Builder
	title := fmt.Sprintf("History (%d)", len(m.historyEntries))
	activeTitleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	titleStr := activeTitleStyle.Render("▸ " + title)
	titleLen := len(title) + 2

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╭─"))
	box.WriteString(titleStr)
	padLen := width - titleLen + 1
	if padLen < 0 {
		padLen = 0
	}
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(strings.Repeat("─", padLen) + "╮"))
	box.WriteString("\n")

	for _, line := range lines {
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("│ "))
		box.WriteString(padRight(line, width))
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(" │"))
		box.WriteString("\n")
	}

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╰" + strings.Repeat("─", width+2) + "╯"))

	return box.String()
}

func (m Model) createBorderedHistoryPreviewBox(width int, height int) string {
	bc := lipgloss.Color("240")

	var lines []string

	if len(m.historyEntries) > 0 && m.historyCursor < len(m.historyEntries) {
		entry := m.historyEntries[m.historyCursor]

		// Project context (truncated)
		if entry.ProjectContext != "" {
			lines = append(lines, dimStyle.Render("<project_context>"))
			plines := strings.Split(entry.ProjectContext, "\n")
			for i, line := range plines {
				if i >= 3 {
					lines = append(lines, dimStyle.Render("  ...truncated"))
					break
				}
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				lines = append(lines, "  "+line)
			}
			lines = append(lines, dimStyle.Render("</project_context>"))
			lines = append(lines, "")
		}

		// Request (full or truncated based on space)
		if entry.Request != "" {
			lines = append(lines, dimStyle.Render("<request>"))
			rlines := strings.Split(entry.Request, "\n")
			maxReqLines := 5
			for i, line := range rlines {
				if i >= maxReqLines {
					lines = append(lines, dimStyle.Render("  ...truncated"))
					break
				}
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				lines = append(lines, "  "+line)
			}
			lines = append(lines, dimStyle.Render("</request>"))
			lines = append(lines, "")
		}

		// Files
		lines = append(lines, dimStyle.Render("<files>"))
		maxFiles := 5
		for i, f := range entry.Files {
			if i >= maxFiles {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... +%d more files", len(entry.Files)-maxFiles)))
				break
			}
			path := f
			if len(path) > width-6 {
				path = "..." + path[len(path)-width+9:]
			}
			lines = append(lines, "  "+path)
		}
		lines = append(lines, dimStyle.Render("</files>"))
	} else {
		lines = append(lines, dimStyle.Render("(select an entry)"))
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Build box
	var box strings.Builder
	title := "Preview"

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╭─"))
	box.WriteString(dimStyle.Render(title))
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(strings.Repeat("─", width-len(title)+1) + "╮"))
	box.WriteString("\n")

	for _, line := range lines {
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("│ "))
		box.WriteString(padRight(line, width))
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(" │"))
		box.WriteString("\n")
	}

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╰" + strings.Repeat("─", width+2) + "╯"))

	return box.String()
}

func (m Model) createBorderedBox(title string, content string, width int, height int, active bool) string {
	borderColor := "240"
	if active {
		borderColor = "14" // bright cyan for active
	}

	// Prepare content lines
	var lines []string
	if content == "" {
		lines = []string{dimStyle.Render("(empty)")}
	} else {
		lines = strings.Split(content, "\n")
	}

	// Truncate/pad to fit
	for i := range lines {
		if len(lines[i]) > width-2 {
			lines[i] = lines[i][:width-5] + "..."
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Build box with border
	var box strings.Builder
	bc := lipgloss.Color(borderColor)

	// Title in top border
	activeTitleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	titleStr := title
	titleLen := len(title)
	if active {
		titleStr = activeTitleStyle.Render("▸ " + title)
		titleLen = len(title) + 2 // account for marker
	} else {
		titleStr = dimStyle.Render(title)
	}
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╭─"))
	box.WriteString(titleStr)
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(strings.Repeat("─", width-titleLen+1) + "╮"))
	box.WriteString("\n")

	// Content lines
	for _, line := range lines {
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("│ "))
		box.WriteString(padRight(line, width))
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(" │"))
		box.WriteString("\n")
	}

	// Bottom border
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╰" + strings.Repeat("─", width+2) + "╯"))

	return box.String()
}

func (m Model) createBorderedFilesBox(width int, height int, active bool) string {
	borderColor := "240"
	if active {
		borderColor = "14" // bright cyan for active
	}

	// Prepare content
	var lines []string
	sizeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan for size
	sizeWidth := 8 // fixed width for size column

	if len(m.files) == 0 {
		lines = []string{dimStyle.Render("(no files)")}
	} else {
		for i, f := range m.files {
			if i >= height {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("... +%d more", len(m.files)-height)))
				break
			}
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}

			// Calculate available width for path (total - prefix - size - spacing)
			pathWidth := width - len(prefix) - sizeWidth - 1
			if pathWidth < 10 {
				pathWidth = 10
			}

			path := f.RelPath
			if len(path) > pathWidth {
				path = "..." + path[len(path)-pathWidth+3:]
			}

			// Pad path to fixed width for table alignment
			paddedPath := path + strings.Repeat(" ", pathWidth-len(path))

			// Format size right-aligned
			size := formatSize(f.Size)
			paddedSize := fmt.Sprintf("%*s", sizeWidth, size)

			// Build line with colored size
			if i == m.cursor {
				line := cursorStyle.Render(prefix + paddedPath) + " " + sizeStyle.Render(paddedSize)
				lines = append(lines, line)
			} else if f.Selected {
				line := selectedStyle.Render(prefix + paddedPath) + " " + sizeStyle.Render(paddedSize)
				lines = append(lines, line)
			} else {
				line := prefix + paddedPath + " " + sizeStyle.Render(paddedSize)
				lines = append(lines, line)
			}
		}
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Build box
	var box strings.Builder
	bc := lipgloss.Color(borderColor)
	title := fmt.Sprintf("Files (%d)", len(m.files))

	activeTitleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	titleStr := title
	titleLen := len(title)
	if active {
		titleStr = activeTitleStyle.Render("▸ " + title)
		titleLen = len(title) + 2
	} else {
		titleStr = dimStyle.Render(title)
	}

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╭─"))
	box.WriteString(titleStr)
	padLen := width - titleLen + 1
	if padLen < 0 {
		padLen = 0
	}
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(strings.Repeat("─", padLen) + "╮"))
	box.WriteString("\n")

	for _, line := range lines {
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("│ "))
		box.WriteString(padRight(line, width))
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(" │"))
		box.WriteString("\n")
	}

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╰" + strings.Repeat("─", width+2) + "╯"))

	return box.String()
}

func (m Model) createBorderedPreviewBox(width int, height int) string {
	bc := lipgloss.Color("240")

	// Build preview content
	var lines []string

	if m.context.ProjectContext != "" {
		lines = append(lines, dimStyle.Render("<project_context>"))
		plines := strings.Split(m.context.ProjectContext, "\n")
		for i, line := range plines {
			if i >= 3 {
				lines = append(lines, dimStyle.Render("  ...truncated"))
				break
			}
			if len(line) > width-4 {
				line = line[:width-7] + "..."
			}
			lines = append(lines, "  "+line)
		}
		lines = append(lines, dimStyle.Render("</project_context>"))
		lines = append(lines, "")
	}

	if m.context.Request != "" {
		lines = append(lines, dimStyle.Render("<request>"))
		rlines := strings.Split(m.context.Request, "\n")
		for _, line := range rlines {
			if len(line) > width-4 {
				line = line[:width-7] + "..."
			}
			lines = append(lines, "  "+line)
		}
		lines = append(lines, dimStyle.Render("</request>"))
		lines = append(lines, "")
	}

	lines = append(lines, dimStyle.Render("<files>"))
	for i, f := range m.files {
		if i >= 5 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... +%d more", len(m.files)-5)))
			break
		}
		path := f.Path
		if len(path) > width-6 {
			path = "..." + path[len(path)-width+9:]
		}
		lines = append(lines, "  "+path)
	}
	lines = append(lines, dimStyle.Render("</files>"))

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Build box
	var box strings.Builder
	title := "Preview"

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╭─"))
	box.WriteString(dimStyle.Render(title))
	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(strings.Repeat("─", width-len(title)+1) + "╮"))
	box.WriteString("\n")

	for _, line := range lines {
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("│ "))
		box.WriteString(padRight(line, width))
		box.WriteString(lipgloss.NewStyle().Foreground(bc).Render(" │"))
		box.WriteString("\n")
	}

	box.WriteString(lipgloss.NewStyle().Foreground(bc).Render("╰" + strings.Repeat("─", width+2) + "╯"))

	return box.String()
}

func (m Model) boxTitle(title string, active bool) string {
	if active {
		return titleStyle.Render(title)
	}
	return dimStyle.Render(title)
}

func (m Model) renderBoxContent(content string, width int, height int, active bool) string {
	if content == "" {
		return dimStyle.Render("(empty)")
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		if len(line) > width-2 {
			lines[i] = line[:width-5] + "..."
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderFilesBoxContent(width int, height int, active bool) string {
	if len(m.files) == 0 {
		return dimStyle.Render("(no files)")
	}

	var lines []string
	for i, f := range m.files {
		if i >= height {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("... +%d more", len(m.files)-height)))
			break
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		path := f.RelPath
		maxLen := width - 15
		if maxLen < 10 {
			maxLen = 10
		}
		if len(path) > maxLen {
			path = "..." + path[len(path)-maxLen+3:]
		}
		line := fmt.Sprintf("%s%s %s", prefix, path, formatSize(f.Size))
		if i == m.cursor {
			line = cursorStyle.Render(line)
		} else if f.Selected {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderPreviewContent(width int, height int) string {
	var lines []string

	// Project context (truncated)
	if m.context.ProjectContext != "" {
		lines = append(lines, dimStyle.Render("<project_context>"))
		plines := strings.Split(m.context.ProjectContext, "\n")
		for i, line := range plines {
			if i >= 3 {
				lines = append(lines, dimStyle.Render("  ...truncated"))
				break
			}
			if len(line) > width-4 {
				line = line[:width-7] + "..."
			}
			lines = append(lines, "  "+line)
		}
		lines = append(lines, dimStyle.Render("</project_context>"))
		lines = append(lines, "")
	}

	// Request (full)
	if m.context.Request != "" {
		lines = append(lines, dimStyle.Render("<request>"))
		rlines := strings.Split(m.context.Request, "\n")
		for _, line := range rlines {
			if len(line) > width-4 {
				line = line[:width-7] + "..."
			}
			lines = append(lines, "  "+line)
		}
		lines = append(lines, dimStyle.Render("</request>"))
		lines = append(lines, "")
	}

	// Files
	lines = append(lines, dimStyle.Render("<files>"))
	for i, f := range m.files {
		if i >= 5 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... +%d more", len(m.files)-5)))
			break
		}
		path := f.Path
		if len(path) > width-6 {
			path = "..." + path[len(path)-width+9:]
		}
		lines = append(lines, "  "+path)
	}
	lines = append(lines, dimStyle.Render("</files>"))

	return strings.Join(lines, "\n")
}

func padRight(s string, length int) string {
	// Account for ANSI escape codes when calculating visible length
	visible := stripAnsi(s)
	if len(visible) >= length {
		return s
	}
	return s + strings.Repeat(" ", length-len(visible))
}

func stripAnsi(s string) string {
	// Simple ANSI stripper - remove escape sequences
	result := ""
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result += string(r)
	}
	return result
}

func (m Model) renderBox(title string, content string, width int, height int, active bool) string {
	// Truncate content to fit
	lines := strings.Split(content, "\n")
	maxLines := height - 2 // Account for border
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	if content == "" {
		lines = []string{dimStyle.Render("(empty)")}
	}

	// Truncate each line to fit width
	for i, line := range lines {
		if len(line) > width-4 {
			lines[i] = line[:width-7] + "..."
		}
	}

	truncatedContent := strings.Join(lines, "\n")

	// Create box style
	borderColor := "8"
	if active {
		borderColor = "12" // Blue for active
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Width(width).
		Height(height - 2).
		Padding(0, 1)

	titleStyle := lipgloss.NewStyle().Bold(true)
	if active {
		titleStyle = titleStyle.Foreground(lipgloss.Color("12"))
	}

	return titleStyle.Render(title) + "\n" + boxStyle.Render(truncatedContent)
}

func (m Model) renderFilesBox(width int, height int, active bool) string {
	var content strings.Builder

	maxLines := height - 3
	if len(m.files) == 0 {
		content.WriteString(dimStyle.Render("(no files)"))
	} else {
		visibleFiles := m.files
		if len(visibleFiles) > maxLines {
			visibleFiles = visibleFiles[:maxLines-1]
		}

		for i, f := range visibleFiles {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}

			// Truncate path
			path := f.RelPath
			maxPathLen := width - 15
			if maxPathLen < 10 {
				maxPathLen = 10
			}
			if len(path) > maxPathLen {
				path = "..." + path[len(path)-maxPathLen+3:]
			}

			line := fmt.Sprintf("%s%s %s", prefix, path, formatSize(f.Size))
			if i == m.cursor {
				line = cursorStyle.Render(line)
			} else if f.Selected {
				line = selectedStyle.Render(line)
			}
			content.WriteString(line + "\n")
		}

		if len(m.files) > maxLines {
			content.WriteString(dimStyle.Render(fmt.Sprintf("  ... +%d more", len(m.files)-maxLines+1)))
		}
	}

	// Create box style
	borderColor := "8"
	if active {
		borderColor = "12"
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Width(width).
		Height(height - 2).
		Padding(0, 1)

	titleStyle := lipgloss.NewStyle().Bold(true)
	if active {
		titleStyle = titleStyle.Foreground(lipgloss.Color("12"))
	}

	title := fmt.Sprintf("Files (%d)", len(m.files))
	return titleStyle.Render(title) + "\n" + boxStyle.Render(content.String())
}

func (m Model) renderPreviewBox(width int, height int) string {
	var content strings.Builder

	// Project context (truncated)
	if m.context.ProjectContext != "" {
		content.WriteString(dimStyle.Render("<project_context>") + "\n")
		lines := strings.Split(m.context.ProjectContext, "\n")
		if len(lines) > 3 {
			for _, line := range lines[:3] {
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				content.WriteString("  " + line + "\n")
			}
			content.WriteString(dimStyle.Render("  ...truncated...") + "\n")
		} else {
			for _, line := range lines {
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				content.WriteString("  " + line + "\n")
			}
		}
		content.WriteString(dimStyle.Render("</project_context>") + "\n\n")
	}

	// Request (full)
	if m.context.Request != "" {
		content.WriteString(dimStyle.Render("<request>") + "\n")
		lines := strings.Split(m.context.Request, "\n")
		for _, line := range lines {
			if len(line) > width-4 {
				line = line[:width-7] + "..."
			}
			content.WriteString("  " + line + "\n")
		}
		content.WriteString(dimStyle.Render("</request>") + "\n\n")
	}

	// Files (truncated)
	content.WriteString(dimStyle.Render("<files>") + "\n")
	maxFiles := 5
	for i, f := range m.files {
		if i >= maxFiles {
			content.WriteString(dimStyle.Render(fmt.Sprintf("  ... +%d more files", len(m.files)-maxFiles)) + "\n")
			break
		}
		path := f.Path
		if len(path) > width-10 {
			path = "..." + path[len(path)-width+13:]
		}
		content.WriteString(fmt.Sprintf("  %s\n", path))
	}
	content.WriteString(dimStyle.Render("</files>") + "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Width(width).
		Height(height - 1).
		Padding(0, 1)

	return lipgloss.NewStyle().Bold(true).Render("Preview") + "\n" + boxStyle.Render(content.String())
}

func (m Model) viewFolders() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("ctx"))
	sb.WriteString(" - ")
	sb.WriteString(m.context.Name)
	sb.WriteString(" ")
	sb.WriteString(dimStyle.Render("[folder view]"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 60)))
	sb.WriteString("\n")

	// Folders header
	sb.WriteString(fmt.Sprintf("Folders (%d):\n", len(m.folders)))

	if len(m.folders) == 0 {
		sb.WriteString(dimStyle.Render("  (no folders)"))
		sb.WriteString("\n")
	} else {
		visibleRows := m.visibleFileRows()
		endIdx := m.folderOffset + visibleRows
		if endIdx > len(m.folders) {
			endIdx = len(m.folders)
		}

		// Show scroll indicator if there are folders above
		if m.folderOffset > 0 {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.folderOffset)))
			sb.WriteString("\n")
		}

		for i := m.folderOffset; i < endIdx; i++ {
			f := m.folders[i]
			prefix := "  "
			if i == m.folderCursor {
				prefix = "> "
			}

			var line strings.Builder
			line.WriteString(prefix)

			if f.Selected {
				line.WriteString("[x] ")
			} else {
				line.WriteString("    ")
			}

			// Folder path (truncated from left if too long)
			path := f.Path
			maxPathLen := 40
			if len(path) > maxPathLen {
				path = "..." + path[len(path)-maxPathLen+3:]
			}
			line.WriteString(fmt.Sprintf("%-40s ", path))

			// File count and size
			line.WriteString(fmt.Sprintf("%3d files  %6s", f.FileCount, formatSize(f.TotalSize)))

			lineStr := line.String()
			if i == m.folderCursor {
				lineStr = cursorStyle.Render(lineStr)
			} else if f.Selected {
				lineStr = selectedStyle.Render(lineStr)
			}

			sb.WriteString(lineStr)
			sb.WriteString("\n")
		}

		// Show scroll indicator if there are folders below
		if endIdx < len(m.folders) {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", len(m.folders)-endIdx)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(strings.Repeat("─", min(m.width, 60)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("[d]elete folder  [space]select  [f]back to files  [q]uit"))
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) viewSelect(title string) string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")

	for i, item := range m.selectItems {
		prefix := "  "
		if i == m.selectCursor {
			prefix = "> "
		}

		line := prefix + item
		if i == m.selectCursor {
			line = cursorStyle.Render(line)
		}

		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")
	// Show delete hint only for context selection
	if strings.Contains(title, "Context") {
		sb.WriteString(dimStyle.Render("[enter] select  [D]elete  [esc] cancel"))
	} else {
		sb.WriteString(dimStyle.Render("[enter] select  [esc] cancel"))
	}
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) viewInput(title string, value string) string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")
	sb.WriteString("> ")
	sb.WriteString(value)
	sb.WriteString("_")
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("[enter] confirm  [esc] cancel"))
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) viewConfig() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Current Config"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Context: %s\n", m.config.ActiveContext))
	sb.WriteString(fmt.Sprintf("Exclude: %s\n", m.config.ActiveExclude))
	sb.WriteString(fmt.Sprintf("Skip prefixes: %v\n", m.config.SkipPrefixes))
	sb.WriteString(strings.Repeat("─", min(m.width, 40)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("[any key] close"))
	sb.WriteString("\n")

	return sb.String()
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	}
	return fmt.Sprintf("%dKB", size/1024)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
