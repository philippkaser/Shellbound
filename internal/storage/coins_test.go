package storage

import (
	"path/filepath"
	"testing"
)

// openTestRepos opens a fresh migrated database in a temp dir.
func openTestRepos(t *testing.T) *Repos {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewRepos(db)
}

func TestCoinsEconomy(t *testing.T) {
	r := openTestRepos(t)
	p, err := r.Players.Create("fp-coins", "trader", "#FFFFFF")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}
	if p.Coins != 0 {
		t.Fatalf("new player should start at 0 coins, got %d", p.Coins)
	}

	// Credit accrues.
	bal, err := r.Players.AddCoins(p.ID, 50)
	if err != nil {
		t.Fatalf("add coins: %v", err)
	}
	if bal != 50 {
		t.Fatalf("balance after +50 = %d, want 50", bal)
	}

	// An affordable purchase debits and reports the new balance.
	ok, bal, err := r.Players.SpendCoins(p.ID, 30)
	if err != nil {
		t.Fatalf("spend coins: %v", err)
	}
	if !ok || bal != 20 {
		t.Fatalf("spend 30 of 50: ok=%v bal=%d, want ok=true bal=20", ok, bal)
	}

	// An unaffordable purchase is refused and leaves the balance intact.
	ok, bal, err = r.Players.SpendCoins(p.ID, 100)
	if err != nil {
		t.Fatalf("spend coins (over): %v", err)
	}
	if ok || bal != 20 {
		t.Fatalf("overspend: ok=%v bal=%d, want ok=false bal=20", ok, bal)
	}

	// Persisted across a fresh read.
	got, err := r.Players.ByID(p.ID)
	if err != nil || got == nil {
		t.Fatalf("reload player: %v", err)
	}
	if got.Coins != 20 {
		t.Fatalf("reloaded balance = %d, want 20", got.Coins)
	}
}

func TestSpendExactBalance(t *testing.T) {
	r := openTestRepos(t)
	p, _ := r.Players.Create("fp-exact", "broke", "#000000")
	if _, err := r.Players.AddCoins(p.ID, 45); err != nil {
		t.Fatalf("add coins: %v", err)
	}
	ok, bal, err := r.Players.SpendCoins(p.ID, 45)
	if err != nil {
		t.Fatalf("spend: %v", err)
	}
	if !ok || bal != 0 {
		t.Fatalf("spend exact balance: ok=%v bal=%d, want ok=true bal=0", ok, bal)
	}
}
