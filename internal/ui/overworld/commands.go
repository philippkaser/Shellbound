package overworld

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/ui/chat"
)

// submitChat handles text sent from the chat bar: plain messages broadcast
// globally; lines starting with "/" run as commands.
func (m Model) submitChat(text string) (Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		return m, nil
	}
	if cmd, ok := chat.ParseCommand(text); ok {
		return m.runCommand(cmd)
	}
	m.handle.Chat(text)
	return m, nil
}

// runCommand executes one slash command.
func (m Model) runCommand(c chat.Command) (Model, tea.Cmd) {
	switch c.Name {
	case "help":
		for _, line := range []string{
			"/help — this list",
			"/who — who's online",
			"/w <user> <msg> — whisper (also /whisper)",
			"/friend add|remove|list <user>",
			"/me <action> — emote",
			"/quit — disconnect",
		} {
			m.chat.AddSystem(line)
		}

	case "who":
		states := m.handle.Who()
		names := make([]string, 0, len(states))
		for _, st := range states {
			names = append(names, st.Info.Name)
		}
		m.chat.AddSystem("online: " + strings.Join(names, ", "))

	case "w", "whisper":
		return m.cmdWhisper(c)

	case "friend":
		return m.cmdFriend(c)

	case "me":
		action := c.ArgsFrom(0)
		if action == "" {
			m.chat.AddSystem("usage: /me <action>")
			return m, nil
		}
		m.handle.Emote(action)

	case "quit":
		return m, func() tea.Msg { return DisconnectMsg{Reason: "bye"} }

	default:
		m.chat.AddSystem("unknown command /" + c.Name + " — try /help")
	}
	return m, nil
}

// cmdWhisper implements /w <user> <message...>.
func (m Model) cmdWhisper(c chat.Command) (Model, tea.Cmd) {
	if len(c.Args) < 2 {
		m.chat.AddSystem("usage: /w <user> <message>")
		return m, nil
	}
	name, text := c.Args[0], c.ArgsFrom(1)
	target, err := m.repos.Players.ByUsername(name)
	if err != nil || target == nil {
		m.chat.AddSystem("no such player: " + name)
		return m, nil
	}
	if target.ID == m.player.ID {
		m.chat.AddSystem("talking to yourself is free, no whisper needed")
		return m, nil
	}
	if err := m.repos.DMs.Save(m.player.ID, target.ID, text); err != nil {
		m.chat.AddSystem("could not send the message")
		return m, nil
	}
	delivered := m.handle.Whisper(target.ID, text)
	m.chat.Add(chat.Entry{Kind: chat.KindWhisperOut, Name: target.Username, Color: target.Color, Text: text})
	if !delivered {
		m.chat.AddSystem(target.Username + " is offline — they'll find it in their messages")
	}
	return m, nil
}

// cmdFriend implements /friend add|remove|list.
func (m Model) cmdFriend(c chat.Command) (Model, tea.Cmd) {
	if len(c.Args) == 0 {
		m.chat.AddSystem("usage: /friend add|remove|list <user>")
		return m, nil
	}
	switch c.Args[0] {
	case "list":
		list, err := m.repos.Friends.List(m.player.ID)
		if err != nil {
			m.chat.AddSystem("could not load friends")
			return m, nil
		}
		if len(list) == 0 {
			m.chat.AddSystem("no friends yet — /friend add <user>")
			return m, nil
		}
		online := m.handle.OnlineIDs()
		names := make([]string, 0, len(list))
		for _, f := range list {
			mark := "○"
			if online[f.ID] {
				mark = "●"
			}
			names = append(names, mark+" "+f.Username)
		}
		m.chat.AddSystem("friends: " + strings.Join(names, "  "))

	case "add", "remove":
		if len(c.Args) < 2 {
			m.chat.AddSystem("usage: /friend " + c.Args[0] + " <user>")
			return m, nil
		}
		name := c.Args[1]
		target, err := m.repos.Players.ByUsername(name)
		if err != nil || target == nil {
			m.chat.AddSystem("no such player: " + name)
			return m, nil
		}
		if target.ID == m.player.ID {
			m.chat.AddSystem("you are already your own best friend")
			return m, nil
		}
		if c.Args[0] == "add" {
			if err := m.repos.Friends.Add(m.player.ID, target.ID); err != nil {
				m.chat.AddSystem("could not add friend")
				return m, nil
			}
			m.toasts.Show("✓ friend added: " + target.Username)
		} else {
			if err := m.repos.Friends.Remove(m.player.ID, target.ID); err != nil {
				m.chat.AddSystem("could not remove friend")
				return m, nil
			}
			m.chat.AddSystem("removed " + target.Username)
		}

	default:
		m.chat.AddSystem("usage: /friend add|remove|list <user>")
	}
	return m, nil
}
