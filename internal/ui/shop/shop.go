// Package shop is the plaza cosmetics stall: a list of the headwear for sale,
// each with its coin price and an "owned" marker, plus the player's current
// balance. Selecting an affordable, unowned piece asks the overworld to buy it;
// the overworld performs the (atomic) charge, grants the item and refreshes the
// panel so the player can keep shopping.
package shop

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/cosmetic"
	"github.com/shellbound/shellbound/internal/style"
	"github.com/shellbound/shellbound/internal/ui/listpanel"
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
	s := key.String()
	if c, ok := listpanel.Nav(s, m.cursor, len(m.stock)); ok {
		m.cursor = c
		return nil
	}
	switch s {
	case "esc", "e", "q":
		m.Close()
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

// Content returns the panel for the pixel renderer; owned rows show "owned"
// in place of a price and "-" marks pieces the player can't afford yet.
func (m *Model) Content() listpanel.Content {
	c := listpanel.Content{
		Title:  "Shop  " + coin + strconv.Itoa(m.balance),
		Cursor: -1,
		Footer: "up/down select  Enter buy  Esc close",
	}
	if len(m.stock) == 0 {
		c.Lines = []string{"Nothing for sale."}
		return c
	}
	for i, it := range m.stock {
		if i >= maxListRows {
			c.Lines = append(c.Lines, "...")
			break
		}
		tail := strconv.Itoa(it.Price) + coin
		mark := " "
		if m.owned[it.Key] {
			mark, tail = "*", "owned"
		} else if it.Price > m.balance {
			mark = "-" // can't afford yet
		}
		if i == m.cursor {
			c.Cursor = len(c.Lines)
		}
		c.Lines = append(c.Lines, mark+" "+listpanel.Pad(it.Name, 16)+listpanel.Pad(it.Rarity.Label(), 10)+tail)
	}
	return c
}
