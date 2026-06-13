package chat

import (
	"reflect"
	"testing"
)

func TestParseCommandPlainText(t *testing.T) {
	for _, in := range []string{"hello", "hello /world", "", "   ", "/", "  /  "} {
		if _, ok := ParseCommand(in); ok {
			t.Errorf("%q should not parse as a command", in)
		}
	}
}

func TestParseCommandBasic(t *testing.T) {
	c, ok := ParseCommand("/help")
	if !ok || c.Name != "help" || len(c.Args) != 0 {
		t.Fatalf("got %+v, ok=%v", c, ok)
	}
}

func TestParseCommandCaseAndArgs(t *testing.T) {
	c, ok := ParseCommand("/W Bob hi there friend")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.Name != "w" {
		t.Errorf("name should lowercase: %q", c.Name)
	}
	want := []string{"Bob", "hi", "there", "friend"}
	if !reflect.DeepEqual(c.Args, want) {
		t.Errorf("args = %v, want %v", c.Args, want)
	}
	if got := c.ArgsFrom(1); got != "hi there friend" {
		t.Errorf("ArgsFrom(1) = %q", got)
	}
	if got := c.ArgsFrom(0); got != "Bob hi there friend" {
		t.Errorf("ArgsFrom(0) = %q", got)
	}
}

func TestParseCommandTrimsWhitespace(t *testing.T) {
	c, ok := ParseCommand("   /who   ")
	if !ok || c.Name != "who" {
		t.Fatalf("got %+v, ok=%v", c, ok)
	}
}

func TestArgsFromOutOfRange(t *testing.T) {
	c, _ := ParseCommand("/me waves")
	if got := c.ArgsFrom(5); got != "" {
		t.Errorf("out-of-range ArgsFrom should be empty, got %q", got)
	}
	if got := c.ArgsFrom(-1); got != "" {
		t.Errorf("negative ArgsFrom should be empty, got %q", got)
	}
}

func TestParseCommandCollapsesSpaces(t *testing.T) {
	c, ok := ParseCommand("/friend   add    Zoe")
	if !ok {
		t.Fatal("expected ok")
	}
	want := []string{"add", "Zoe"}
	if !reflect.DeepEqual(c.Args, want) {
		t.Errorf("args = %v, want %v", c.Args, want)
	}
}
