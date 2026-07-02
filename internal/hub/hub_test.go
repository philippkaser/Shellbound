package hub

import (
	"testing"
	"time"
)

func info(id int64, name string) PlayerInfo {
	return PlayerInfo{ID: id, Name: name, Color: "#FFFFFF"}
}

// drain empties a handle's event buffer and returns everything received.
func drain(h *Handle) []Event {
	var out []Event
	for {
		select {
		case ev, ok := <-h.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestJoinSnapshotAndAnnounce(t *testing.T) {
	h := New()
	a, snapA := h.Join("sid-a", info(1, "ada"), Pos{X: 1, Y: 1})
	if len(snapA) != 0 {
		t.Fatalf("first join saw %d players, want 0", len(snapA))
	}
	_, snapB := h.Join("sid-b", info(2, "bob"), Pos{X: 2, Y: 2})
	if len(snapB) != 1 || snapB[0].Info.ID != 1 {
		t.Fatalf("second join snapshot = %+v, want ada only", snapB)
	}
	evs := drain(a)
	if len(evs) != 1 {
		t.Fatalf("ada got %d events, want 1 join", len(evs))
	}
	if j, ok := evs[0].(EvJoin); !ok || j.State.Info.ID != 2 {
		t.Fatalf("ada's event = %#v, want EvJoin{bob}", evs[0])
	}
}

func TestTakeoverKicksOldSession(t *testing.T) {
	h := New()
	old, _ := h.Join("sid-old", info(1, "ada"), Pos{})
	h.Join("sid-new", info(1, "ada"), Pos{})

	evs := drain(old)
	if len(evs) == 0 {
		t.Fatal("old session got no events, want EvKick")
	}
	if _, ok := evs[0].(EvKick); !ok {
		t.Fatalf("old session's first event = %#v, want EvKick", evs[0])
	}
	// The old channel must be closed.
	if _, ok := <-old.Events(); ok {
		t.Fatal("old session's channel still open after takeover")
	}
}

func TestMoveSweepCoalesces(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	drain(a)
	drain(b)

	// Two rapid moves coalesce into one state in the next sweep.
	a.Move(Pos{X: 5, Y: 5}, DirLeft, true)
	a.Move(Pos{X: 6, Y: 5}, DirRight, true)
	h.sweep()

	evs := drain(b)
	if len(evs) != 1 {
		t.Fatalf("bob got %d events, want 1 coalesced EvMoves", len(evs))
	}
	mv, ok := evs[0].(EvMoves)
	if !ok || len(mv.States) != 1 {
		t.Fatalf("bob's event = %#v, want EvMoves with one state", evs[0])
	}
	if mv.States[0].Pos.X != 6 || mv.States[0].Dir != DirRight {
		t.Fatalf("coalesced state = %+v, want the latest move", mv.States[0])
	}
	// A sweep with nothing dirty broadcasts nothing.
	h.sweep()
	if evs := drain(b); len(evs) != 0 {
		t.Fatalf("idle sweep still broadcast %d events", len(evs))
	}
}

func TestSelfChallengeAndWhisperRejected(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	if ok, reason := a.ChallengePvP(1, nil); ok || reason == "" {
		t.Fatalf("self-challenge accepted (ok=%v reason=%q)", ok, reason)
	}
	if a.Whisper(1, "hi me") {
		t.Fatal("self-whisper reported as delivered")
	}
}

func TestChallengeDeclineNotifiesChallenger(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	drain(a)
	drain(b)

	if ok, reason := a.ChallengePvP(2, nil); !ok {
		t.Fatalf("challenge rejected: %s", reason)
	}
	if evs := drain(b); len(evs) != 1 {
		t.Fatalf("bob got %d events, want the challenge", len(evs))
	}
	b.RespondPvP(1, false, nil)
	evs := drain(a)
	if len(evs) != 1 {
		t.Fatalf("ada got %d events, want a decline", len(evs))
	}
	if _, ok := evs[0].(EvBattleDeclined); !ok {
		t.Fatalf("ada's event = %#v, want EvBattleDeclined", evs[0])
	}
}

func TestAcceptStartsMatchForBothSides(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	drain(a)
	drain(b)

	a.ChallengePvP(2, nil)
	drain(b)
	b.RespondPvP(1, true, nil)

	evA, evB := drain(a), drain(b)
	if len(evA) != 1 || len(evB) != 1 {
		t.Fatalf("got %d/%d events, want 1 battle start each", len(evA), len(evB))
	}
	sa, okA := evA[0].(EvBattleStart)
	sb, okB := evB[0].(EvBattleStart)
	if !okA || !okB {
		t.Fatalf("events %#v / %#v, want EvBattleStart both sides", evA[0], evB[0])
	}
	if sa.Match == nil || sa.Match != sb.Match {
		t.Fatal("the two sides must share one Match")
	}
	if !sa.SideA || sb.SideA {
		t.Fatal("challenger must be side A, responder side B")
	}
	// Both are now busy: a third player can't challenge either.
	c, _ := h.Join("sid-c", info(3, "cyd"), Pos{})
	if ok, _ := c.ChallengePvP(1, nil); ok {
		t.Fatal("challenged a player who is in a battle")
	}
}

func TestAcceptFailsWhenChallengerWentBusy(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	drain(a)
	drain(b)

	a.ChallengePvP(2, nil)
	drain(b)
	a.SetBusy(true) // ada walked into a portal world before bob answered
	b.RespondPvP(1, true, nil)

	evB := drain(b)
	if len(evB) != 1 {
		t.Fatalf("bob got %d events, want a decline", len(evB))
	}
	if _, ok := evB[0].(EvBattleDeclined); !ok {
		t.Fatalf("bob's event = %#v, want EvBattleDeclined", evB[0])
	}
	if evs := drain(a); len(evs) != 0 {
		t.Fatalf("ada should get nothing (she's in a world), got %#v", evs)
	}
}

func TestExpiredChallengeNotifiesChallenger(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	drain(a)
	drain(b)

	a.ChallengePvP(2, nil)
	drain(b)

	// Age the pending challenge past the TTL, then run a sweep.
	h.mu.Lock()
	pc := h.challenges[2]
	pc.at = time.Now().Add(-challengeTTL - time.Second)
	h.challenges[2] = pc
	h.lastExpiry = time.Time{}
	h.mu.Unlock()
	h.sweep()

	evs := drain(a)
	if len(evs) != 1 {
		t.Fatalf("ada got %d events, want an expiry decline", len(evs))
	}
	if _, ok := evs[0].(EvBattleDeclined); !ok {
		t.Fatalf("ada's event = %#v, want EvBattleDeclined", evs[0])
	}
	// The stale challenge is gone: accepting now is a no-op.
	b.RespondPvP(1, true, nil)
	if evs := drain(b); len(evs) != 0 {
		t.Fatalf("expired accept still produced %#v", evs)
	}
}

func TestClobberedChallengeNotifiesFirstChallenger(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	c, _ := h.Join("sid-c", info(3, "cyd"), Pos{})
	drain(a)
	drain(b)
	drain(c)

	a.ChallengePvP(2, nil)
	c.ChallengePvP(2, nil) // cyd's challenge replaces ada's
	evs := drain(a)
	if len(evs) != 1 {
		t.Fatalf("ada got %d events, want a displaced notice", len(evs))
	}
	if _, ok := evs[0].(EvBattleDeclined); !ok {
		t.Fatalf("ada's event = %#v, want EvBattleDeclined", evs[0])
	}
}

func TestLeaveDropsChallengesAndNotifiesOthers(t *testing.T) {
	h := New()
	a, _ := h.Join("sid-a", info(1, "ada"), Pos{})
	b, _ := h.Join("sid-b", info(2, "bob"), Pos{})
	drain(a)
	drain(b)

	a.ChallengePvP(2, nil)
	drain(b)
	a.Leave()

	// bob hears the leave; accepting the orphaned challenge is a no-op.
	evs := drain(b)
	if len(evs) != 1 {
		t.Fatalf("bob got %d events, want the leave", len(evs))
	}
	if lv, ok := evs[0].(EvLeave); !ok || lv.PlayerID != 1 {
		t.Fatalf("bob's event = %#v, want EvLeave{ada}", evs[0])
	}
	b.RespondPvP(1, true, nil)
	if evs := drain(b); len(evs) != 0 {
		t.Fatalf("orphaned accept still produced %#v", evs)
	}
	// Leave is idempotent.
	a.Leave()
}
