package hub

// PlayerInfo is the public identity broadcast to other sessions.
type PlayerInfo struct {
	ID       int64
	Name     string
	Color    string // hex personal color
	Cosmetic string // equipped headwear key ("" = bare-headed)
}

// Pos is a plaza position: X is the cell column of the feet, Y the
// half-block pixel row of the feet (two pixel rows per cell row).
type Pos struct {
	X, Y int
}

// Dir is a 4-way facing direction. Values intentionally mirror
// sprites.Facing so the renderer can convert with a cast.
type Dir int

// Dir values.
const (
	DirDown Dir = iota
	DirUp
	DirLeft
	DirRight
)

// PlayerState is one player's live state as seen by other sessions.
type PlayerState struct {
	Info   PlayerInfo
	Pos    Pos
	Dir    Dir
	Moving bool
}

// Event is anything the hub fans out to sessions. The concrete types below
// are the full set.
type Event interface{ isEvent() }

// EvJoin announces a player entering the plaza.
type EvJoin struct{ State PlayerState }

// EvLeave announces a player leaving.
type EvLeave struct {
	PlayerID int64
	Name     string
}

// EvMoves carries the coalesced position updates from one broadcast sweep
// (at most one entry per player per sweep). Receivers should ignore their
// own entry.
type EvMoves struct{ States []PlayerState }

// EvChat is a global chat line or /me emote.
type EvChat struct {
	From  PlayerInfo
	Text  string
	Emote bool
}

// EvWhisper is a direct message delivered live.
type EvWhisper struct {
	From PlayerInfo
	Text string
}

// EvKick tells a session it has been replaced (same player connected from
// elsewhere). The session's event channel closes right after.
type EvKick struct{ Reason string }

// EvEmote announces that a player played a gesture (its Kind is an emote key).
// Receivers render it on that player's avatar; the sender hears it too, so a
// single path drives self and peers alike.
type EvEmote struct {
	PlayerID int64
	Kind     string
}

func (EvJoin) isEvent()    {}
func (EvLeave) isEvent()   {}
func (EvMoves) isEvent()   {}
func (EvChat) isEvent()    {}
func (EvWhisper) isEvent() {}
func (EvKick) isEvent()    {}
func (EvEmote) isEvent()   {}
