package chat

import "strings"

// Command is a parsed slash command.
type Command struct {
	// Name is the command word, lowercased, without the slash ("help",
	// "w", "friend").
	Name string
	// Args are the whitespace-separated arguments after the name.
	Args []string
}

// ArgsFrom joins Args[i:] with single spaces — used for free-text tails
// like the message body of /w or /me. Returns "" when i is out of range.
func (c Command) ArgsFrom(i int) string {
	if i < 0 || i >= len(c.Args) {
		return ""
	}
	return strings.Join(c.Args[i:], " ")
}

// ParseCommand interprets input as a slash command. It returns ok=false
// for plain chat messages, the empty string, or a bare "/".
func ParseCommand(input string) (Command, bool) {
	s := strings.TrimSpace(input)
	if len(s) < 2 || s[0] != '/' {
		return Command{}, false
	}
	fields := strings.Fields(s[1:])
	if len(fields) == 0 {
		return Command{}, false
	}
	return Command{
		Name: strings.ToLower(fields[0]),
		Args: fields[1:],
	}, true
}
