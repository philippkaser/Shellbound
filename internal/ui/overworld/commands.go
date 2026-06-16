package overworld

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shellbound/shellbound/internal/storage"
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
			"/w <user> — show recent messages with them",
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

// cmdWhisper implements /w <user> [<message...>]. With a message it sends a
// DM; with just a name it prints the recent thread into the console.
func (m Model) cmdWhisper(c chat.Command) (Model, tea.Cmd) {
	if len(c.Args) == 0 {
		m.chat.AddSystem("usage: /w <user> <message>  ·  /w <user> reads recent messages")
		return m, nil
	}
	name := c.Args[0]
	target, err := m.repos.Players.ByUsername(name)
	if err != nil || target == nil {
		m.chat.AddSystem("no such player: " + name)
		return m, nil
	}
	if target.ID == m.player.ID {
		m.chat.AddSystem("talking to yourself is free, no whisper needed")
		return m, nil
	}
	if len(c.Args) == 1 {
		return m.showConversation(*target)
	}
	text := c.ArgsFrom(1)
	if err := m.repos.DMs.Save(m.player.ID, target.ID, text); err != nil {
		m.chat.AddSystem("could not send the message")
		return m, nil
	}
	delivered := m.handle.Whisper(target.ID, text)
	m.chat.Add(chat.Entry{Kind: chat.KindWhisperOut, Name: target.Username, Color: target.Color, Text: text})
	if !delivered {
		m.chat.AddSystem(target.Username + " is offline — the message is saved for them")
	}
	return m, nil
}

// showConversation replays the recent DM thread with target into the chat
// console as whisper lines, newest last. It clears any unread badge for
// that player since the messages are now on screen.
func (m Model) showConversation(target storage.Player) (Model, tea.Cmd) {
	msgs, err := m.repos.DMs.Conversation(m.player.ID, target.ID, 10)
	if err != nil {
		m.chat.AddSystem("could not load messages with " + target.Username)
		return m, nil
	}
	if len(msgs) == 0 {
		m.chat.AddSystem("no messages with " + target.Username + " yet — /w " + target.Username + " <message>")
		return m, nil
	}
	m.chat.AddSystem("— recent with " + target.Username + " —")
	for _, dm := range msgs {
		kind := chat.KindWhisperIn
		if dm.SenderID == m.player.ID {
			kind = chat.KindWhisperOut
		}
		m.chat.Add(chat.Entry{Kind: kind, Name: target.Username, Color: target.Color, Text: dm.Body})
	}
	delete(m.unread, target.ID)
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
