package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	composerMinLines = 1
	composerMaxLines = 8
)

// ComposerContext mirrors the row of chips the app shows around its composer.
type ComposerContext struct {
	Project     string
	Environment string
	Provider    string
	Model       string
	Reasoning   string
	Permission  string
}

// ContextHook supplies those chips. Without it the composer shows only the
// input, which is what it did before and is still correct for TaskYou.
var ContextHook func(taskID int64) ComposerContext

// prettyModel turns a bb model id into what the app puts on the chip:
// "claude-opus-5[1m]" reads as "Opus 5 1M".
func prettyModel(id string) string {
	if id == "" {
		return ""
	}
	name := strings.TrimPrefix(id, "claude-")
	name = strings.TrimPrefix(name, "gpt-")
	suffix := ""
	if index := strings.Index(name, "["); index >= 0 {
		suffix = strings.ToUpper(strings.Trim(name[index:], "[]"))
		name = name[:index]
	}
	parts := strings.Split(strings.Trim(name, "-"), "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	label := strings.Join(parts, " ")
	if suffix != "" {
		label += " " + suffix
	}
	return label
}

// prettyPermission uses bb's own wording for its permission modes.
// PrettyPermission exposes bb's own wording for a permission mode.
func PrettyPermission(mode string) string { return prettyPermission(mode) }

func prettyPermission(mode string) string {
	switch mode {
	case "full":
		return "Full Access"
	case "auto":
		return "Auto"
	case "accept-edits":
		return "Accept Edits"
	case "default":
		return "Ask"
	}
	return mode
}

// ActivityHook reports whether the thread is mid-turn and what it is doing, so
// the view can say so instead of looking idle while bb works.
var ActivityHook func(taskID int64) (bool, string)

var thinkingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func thinkingFrame() string {
	if !SupportsUnicode() {
		return "*"
	}
	return thinkingFrames[(time.Now().UnixMilli()/100)%int64(len(thinkingFrames))]
}

// ComposerEnabled turns on the message box at the bottom of the detail view.
// TaskYou has no composer because a task is driven by typing into its tmux
// pane; a bb thread is driven by sending it a message, so the detail view needs
// somewhere to write one.
var ComposerEnabled = false

type composer struct {
	input              textarea.Model
	focused            bool
	ready              bool
	menu               slashMenu
	attachments        []Attachment
	overrideModel      string
	overridePermission string
	context            ComposerContext
	sending            bool
	working            bool
	workingLabel       string
	workingFor         string
	err                string
}

func newComposer(width int) composer {
	input := textarea.New()
	input.Placeholder = "Ask for a follow-up. @ to mention files, / for skills"
	input.ShowLineNumbers = false
	input.CharLimit = 0
	input.SetWidth(maxInt(20, width-6))
	input.SetHeight(composerMinLines)

	// bubbles ships a highlighted cursor line and a bordered base; both read as
	// a grey slab behind the text in a box this short, so they are cleared and
	// the surrounding border carries the focus state instead.
	for _, style := range []*textarea.Style{&input.FocusedStyle, &input.BlurredStyle} {
		style.Base = lipgloss.NewStyle()
		style.CursorLine = lipgloss.NewStyle()
		style.CursorLineNumber = lipgloss.NewStyle()
		style.EndOfBuffer = lipgloss.NewStyle()
		style.LineNumber = lipgloss.NewStyle()
		style.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)
		style.Prompt = lipgloss.NewStyle().Foreground(ColorPrimary)
		style.Text = lipgloss.NewStyle()
	}
	input.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(ColorMuted)
	input.Prompt = Icon("› ", "> ")

	// Starts unfocused: the thread view keeps TaskYou's single-key actions, and
	// a composer that grabs the keyboard on open makes every one of them
	// unreachable.
	input.Blur()
	return composer{input: input, focused: false, ready: true}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *composer) setWidth(width int) {
	if !c.ready {
		return
	}
	c.input.SetWidth(maxInt(20, width-6))
	c.resize()
}

// resize grows the box with the message, the way a chat composer does, and
// stops at composerMaxLines so a long message cannot squeeze the conversation
// off the screen.
func (c *composer) resize() {
	if !c.ready {
		return
	}
	// Count wrapped lines, not just explicit ones: a long single-line message
	// occupies several rows in the box and the box has to grow with it.
	width := maxInt(1, c.input.Width())
	lines := 0
	for _, paragraph := range strings.Split(c.input.Value(), "\n") {
		rows := (len([]rune(paragraph)) / width) + 1
		lines += rows
	}
	if lines < composerMinLines {
		lines = composerMinLines
	}
	if lines > composerMaxLines {
		lines = composerMaxLines
	}
	if c.input.Height() != lines {
		c.input.SetHeight(lines)
	}
}

// height is what the thread view must reserve: the input, plus the activity
// line above it and the hint below.
func (c *composer) height() int {
	if !c.ready {
		return 0
	}
	c.resize()
	// input + its border + the context row, plus the status line only when it is
	// actually showing.
	height := c.input.Height() + 3 + c.menu.height()
	if c.working {
		height++
	}
	if len(c.attachments) > 0 {
		height++
	}
	return height
}

func (c *composer) focus() {
	c.focused = true
	c.input.Focus()
}

func (c *composer) blur() {
	c.focused = false
	c.input.Blur()
}

func (c *composer) value() string {
	return strings.TrimSpace(c.input.Value())
}

func (c *composer) reset() {
	c.input.Reset()
}

// render draws the composer: an obvious input box, a one-line status above it
// while the thread is working, and a single row underneath saying where the
// message goes and what will answer it.
func (c *composer) render(width int) string {
	if !c.ready {
		return ""
	}

	var parts []string
	if menu := c.menu.render(width); menu != "" {
		parts = append(parts, menu)
	}
	if row := c.attachmentRow(); row != "" {
		parts = append(parts, row)
	}
	if status := c.statusLine(); status != "" {
		parts = append(parts, status)
	}

	border := ColorMuted
	if c.focused {
		border = ColorPrimary
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxInt(20, width-2))

	parts = append(parts, box.Render(c.input.View()), c.chips(width))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// statusLine says the thread is working without repeating the log line above
// it. The command is already on screen; what is missing is that it is still
// running and for how long.
func (c *composer) statusLine() string {
	if !c.working {
		return ""
	}
	label := "  " + thinkingFrame() + " working"
	if c.workingFor != "" {
		label += " · " + c.workingFor
	}
	running := lipgloss.NewStyle().Foreground(ColorInProgress).Render(label)

	// The app puts a stop control on the composer while a turn is in flight.
	// A terminal has no button, so the key is named where the button would be.
	stop := lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render("ctrl+s") +
		lipgloss.NewStyle().Foreground(ColorMuted).Render(" stop")
	return running + "   " + stop
}

func (c *composer) update(msg tea.Msg) (tea.Cmd, bool) {
	if !c.ready || !c.focused {
		return nil, false
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	c.resize()
	return cmd, true
}

// composerOwnsKey reports whether a keystroke is text the composer should
// swallow. Anything else — navigation, scrolling, modified keys — falls through
// to the thread view, so typing a message never costs access to its shortcuts.
func composerOwnsKey(key tea.KeyMsg) bool {
	switch key.Type {
	case tea.KeyRunes:
		// alt+<letter> is a shortcut, not text.
		return !key.Alt

	case tea.KeySpace, tea.KeyBackspace, tea.KeyDelete,
		tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd:
		// These stay text edits with a modifier held: option+delete deletes a
		// word, option+arrow moves one. Rejecting them when Alt was set is what
		// made macOS editing keys fall through to the board.
		return true

	case tea.KeyCtrlA, tea.KeyCtrlE, tea.KeyCtrlU, tea.KeyCtrlW, tea.KeyCtrlK,
		tea.KeyCtrlB, tea.KeyCtrlF, tea.KeyCtrlD, tea.KeyCtrlH,
		tea.KeyCtrlLeft, tea.KeyCtrlRight,
		tea.KeyShiftLeft, tea.KeyShiftRight, tea.KeyShiftHome, tea.KeyShiftEnd:
		// Emacs-style line editing, which the textarea binds, plus the word and
		// selection movements terminals send as their own key types.
		return true
	}
	return false
}

// hashString folds extra text into an existing view-cache signature.
func hashString(seed uint64, value string) uint64 {
	hash := seed
	for _, r := range value {
		hash ^= uint64(r)
		hash *= 1099511628211
	}
	return hash
}

// chips names where the message goes and what will answer it, in words. The
// earlier version used ◇ and ⎇ glyphs, which say nothing to a reader who has
// not been told what they mean.
func (c *composer) chips(width int) string {
	dim := lipgloss.NewStyle().Foreground(ColorMuted)
	sep := dim.Render(" · ")

	var left []string
	if c.context.Project != "" {
		left = append(left, dim.Render(c.context.Project))
	}
	if c.context.Environment != "" {
		left = append(left, dim.Render(c.context.Environment))
	}

	var right []string
	if model := c.context.Model; model != "" {
		label := model
		if c.context.Reasoning != "" {
			label += " " + title(c.context.Reasoning)
		}
		right = append(right, dim.Render(label))
	}
	if permission := prettyPermission(c.context.Permission); permission != "" {
		style := dim
		if c.context.Permission == "full" {
			style = lipgloss.NewStyle().Foreground(ColorWarning)
		}
		right = append(right, style.Render(permission))
	}

	if len(left) == 0 && len(right) == 0 {
		return ""
	}

	leftText := "  " + strings.Join(left, sep)
	rightText := strings.Join(right, sep) + "  "
	gap := width - lipgloss.Width(leftText) - lipgloss.Width(rightText)
	if gap < 1 {
		return leftText
	}
	return leftText + strings.Repeat(" ", gap) + rightText
}

func title(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

// syncMenu opens the slash menu when the message is a bare "/..." token and
// closes it otherwise, so the menu tracks what has been typed without needing
// its own key handling for every character.
func (c *composer) syncMenu(items func(MenuKind, string) []SlashItem) {
	value := c.input.Value()
	token := lastToken(value)

	kind := MenuKind("")
	switch {
	case strings.HasPrefix(token, "@"):
		kind = MenuMentions
	case strings.HasPrefix(token, "/") && token == strings.TrimSpace(value):
		// "/" only opens skills as the whole message, matching the app: a slash
		// inside a sentence is a path, not a command.
		kind = MenuSkills
	}

	if kind == "" {
		c.menu.open = false
		return
	}

	query := token[1:]
	reopened := !c.menu.open || c.menu.kind != kind
	if reopened {
		c.menu.open = true
		c.menu.kind = kind
		c.menu.index = 0
	}
	// Mentions are searched server-side, so the list is refetched as the query
	// changes; skills are fetched once and filtered here.
	if items != nil && (reopened || kind == MenuMentions) {
		c.menu.items = items(kind, query)
	}
	c.menu.query = query
	c.menu.refilter()
}

// lastToken is the whitespace-delimited word the cursor is in, which is what a
// trigger applies to.
func lastToken(value string) string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t'
	})
	if len(fields) == 0 {
		return ""
	}
	if strings.HasSuffix(value, " ") || strings.HasSuffix(value, "\n") {
		return ""
	}
	return fields[len(fields)-1]
}

// acceptMenu replaces the typed token with the chosen skill.
func (c *composer) acceptMenu() bool {
	item, ok := c.menu.selected()
	if !ok {
		return false
	}
	trigger := "/"
	if c.menu.kind == MenuMentions {
		trigger = "@"
	}
	value := c.input.Value()
	token := lastToken(value)
	replacement := trigger + item.Name + " "
	c.input.SetValue(strings.TrimSuffix(value, token) + replacement)
	c.input.CursorEnd()
	c.menu.open = false
	return true
}

func (c *composer) menuOpen() bool { return c.menu.open }

func (c *composer) menuMove(delta int) { c.menu.move(delta) }

func (c *composer) closeMenu() { c.menu.open = false }

// captureAttachments turns any path dropped into the message into a real
// attachment and leaves a short placeholder behind, rather than a screenful of
// backslash-escaped path.
func (c *composer) captureAttachments() {
	value := c.input.Value()
	if !strings.Contains(value, "/") {
		return
	}
	replaced, found := ExtractAttachments(value, len(c.attachments))
	if len(found) == 0 {
		return
	}
	c.attachments = append(c.attachments, found...)
	c.input.SetValue(replaced)
	c.input.CursorEnd()
}

// attachmentRow lists what will be sent with the message.
func (c *composer) attachmentRow() string {
	if len(c.attachments) == 0 {
		return ""
	}
	var chips []string
	for i, attachment := range c.attachments {
		glyph := Icon("📄", "[f]")
		if attachment.Image {
			glyph = Icon("🖼", "[i]")
		}
		chips = append(chips, lipgloss.NewStyle().Foreground(ColorSecondary).
			Render(glyph+" "+strconv.Itoa(i+1)+" "+truncateLine(attachment.Name, 28)))
	}
	hint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  ctrl+x clears")
	return "  " + strings.Join(chips, lipgloss.NewStyle().Foreground(ColorMuted).Render("  ")) + hint
}

func (c *composer) attachPath(path string) {
	_, found := ExtractAttachments(path, len(c.attachments))
	if len(found) == 0 {
		return
	}
	c.attachments = append(c.attachments, found...)
}

func (c *composer) takeAttachments() []Attachment {
	out := c.attachments
	c.attachments = nil
	return out
}

func (c *composer) clearAttachments() {
	c.attachments = nil
}

// openPicker shows a chooser that is not driven by what has been typed: the
// model and permission menus are opened by a key, and their selection changes
// the composer rather than the message.
func (c *composer) openPicker(kind MenuKind, items []SlashItem, current string) {
	c.menu.open = true
	c.menu.kind = kind
	c.menu.query = ""
	c.menu.items = items
	c.menu.refilter()
	c.menu.index = 0
	for i, item := range c.menu.filtered {
		if item.Name == current {
			c.menu.index = i
			break
		}
	}
}

// isPicker reports whether the open menu changes the composer rather than
// inserting into the message.
func (c *composer) isPicker() bool {
	return c.menu.open && (c.menu.kind == MenuModels || c.menu.kind == MenuPermissions)
}

func (c *composer) pickerKind() MenuKind { return c.menu.kind }

func (c *composer) pickedItem() (SlashItem, bool) { return c.menu.selected() }

// arrowPairLabel names the up/down keys in help text. The ASCII fallback for an
// arrow is "^", which in a key hint reads as ctrl — so without Unicode the keys
// are spelled out instead.
func arrowPairLabel() string {
	if SupportsUnicode() {
		return IconArrowUp() + "/" + IconArrowDown()
	}
	return "up/down"
}

// ArrowPairLabel is arrowPairLabel for callers outside this file.
func ArrowPairLabel() string { return arrowPairLabel() }
