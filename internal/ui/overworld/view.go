package overworld

// View returns a constant sentinel. The plaza never renders through bubbletea:
// a background Renderer (renderer.go) bakes full frames into a pixel canvas and
// ships them as Sixel images straight to the session. Returning the same string
// every call keeps bubbletea's renderer quiescent so it never clobbers our
// graphics.
func (m Model) View() string { return " " }
