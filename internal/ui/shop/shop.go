// Package shop is the plaza cosmetics stall: a list of the headwear for sale,
// each with its coin price and an "owned" marker, plus the player's current
// balance. Selecting an affordable, unowned piece asks the overworld to buy it;
// the overworld performs the (atomic) charge, grants the item and refreshes the
// panel so the player can keep shopping.
package shop

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/cosmetic"
	"github.com/shellbound/shellbound/internal/style"
)

const maxListRows = 12

// coin is the in-font icon used for prices and the balance (the same spark the
// portals and reward items use).
const coin = "✦"

// Model is the shop panel.
type Model struct {
	theme   style.Theme
	open    bool
	stock   []cosmetic.Cosmetic
	owned   map[string]bool
	balance int
	cursor  int

	// pending is set when the player presses Enter on an unowned piece; the
	// overworld drains it via TakePurchase to charge and grant.
	pending *string
}

// New creates a closed shop panel.
func New(theme style.Theme) Model { return Model{theme: theme} }

// IsOpen reports whether the panel is showing.
func (m *Model) IsOpen() bool { return m.open }

// Open shows the shop with the full buyable stock, the set of cosmetic keys the
// player already owns, and their coin balance.
func (m *Model) Open(owned map[string]bool, balance int) {
	m.open = true
	m.stock = cosmetic.Shop()
	m.owned = owned
	m.balance = balance
	m.cursor = 0
}

// SetState refreshes ownership and balance in place (after a purchase) without
// disturbing the cursor, so the panel stays put while the player keeps buying.
func (m *Model) SetState(owned map[string]bool, balance int) {
	m.owned = owned
	m.balance = balance
}

// Close hides the panel.
func (m *Model) Close() { m.open = false }

// TakePurchase returns and clears a pending purchase key.
func (m *Model) TakePurchase() (string, bool) {
	if m.pending == nil {
		return "", false
	}
	k := *m.pending
	m.pending = nil
	return k, true
}

// Update handles navigation; Enter requests buying the highlighted piece (the
// overworld decides whether it's affordable). Already-owned rows do nothing.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if !m.open {
		return nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc", "e", "q":
		m.Close()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.stock)-1 {
			m.cursor++
		}
	case "enter":
		if m.cursor < len(m.stock) {
			c := m.stock[m.cursor]
			if !m.owned[c.Key] {
				k := c.Key
				m.pending = &k
			}
		}
	}
	return nil
}

// Lines returns the panel content for the pixel renderer. The highlighted row
// is marked "> "; owned rows show "owned" in place of a price.
func (m *Model) Lines() []string {
	out := []string{"Shop  " + coin + strconv.Itoa(m.balance), ""}
	if len(m.stock) == 0 {
		out = append(out, "Nothing for sale.")
	}
	for i, c := range m.stock {
		if i >= maxListRows {
			out = append(out, "...")
			break
		}
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		tail := strconv.Itoa(c.Price) + coin
		mark := " "
		if m.owned[c.Key] {
			mark, tail = "*", "owned"
		} else if c.Price > m.balance {
			mark = "-" // can't afford yet
		}
		out = append(out, cursor+mark+" "+pad(c.Name, 16)+pad(c.Rarity.Label(), 10)+tail)
	}
	return append(out, "", "up/down select  Enter buy  Esc close")
}

// pad right-pads s with spaces to at least n columns so the price column lines
// up in the fixed-width panel font.
func pad(s string, n int) string {
	if len(s) >= n {
		return s + " "
	}
	return s + strings.Repeat(" ", n-len(s))
}

// View renders the panel box (unused on the Sixel path, kept for parity).
func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.theme.PanelTitle.Render("Shop"))
	b.WriteString("\n\n")
	for i, c := range m.stock {
		line := "  " + c.Name + "  " + strconv.Itoa(c.Price) + coin
		if i == m.cursor {
			line = "> " + c.Name + "  " + strconv.Itoa(c.Price) + coin
		}
		if m.owned[c.Key] {
			line += " (owned)"
		}
		b.WriteString(m.theme.Text.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Faded.Render("up/down select  Enter buy  Esc close"))
	return m.theme.PanelBorder.Width(44).Render(b.String())
}
