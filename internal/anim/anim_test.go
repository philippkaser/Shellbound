package anim

import "testing"

func TestCameraAdvanceApproachesTarget(t *testing.T) {
	c := NewCamera(0, 0)
	c.Advance(100, 0, 1.0/30) // one fixed step
	x1 := c.X
	if x1 <= 0 || x1 >= 100 {
		t.Fatalf("one step should move partway, got %v", x1)
	}
	c.Advance(100, 0, 1.0/30)
	if c.X <= x1 {
		t.Fatalf("camera should keep approaching: %v then %v", x1, c.X)
	}
}

func TestCameraAdvanceCapsHugeDelta(t *testing.T) {
	// A long pause must not teleport the camera across the map in one call.
	c := NewCamera(0, 0)
	c.Advance(1000, 0, 100.0)
	if c.X >= 1000 {
		t.Fatalf("capped advance should not reach the target instantly, got %v", c.X)
	}
}

func TestCameraSnapResets(t *testing.T) {
	c := NewCamera(0, 0)
	c.Advance(100, 0, 0.2)
	c.Snap(5, 7)
	if c.X != 5 || c.Y != 7 || c.vx != 0 || c.vy != 0 {
		t.Fatalf("Snap should hard-set position and zero velocity, got %+v", *c)
	}
}
