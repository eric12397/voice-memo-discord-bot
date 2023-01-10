package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	token string
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.Parse()
}

func main() {
	// Create discord session.
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating a Discord session: ", err)
		return
	}

	voiceMemoManager, err := NewVoiceMemoManager()
	if err != nil {
		fmt.Println("Error creating Voice Memo Manager for Discord session: ", err)
		return
	}
	voiceMemoManager.LoadAll()

	bot, err := NewBot(voiceMemoManager)
	if err != nil {
		fmt.Println("Error creating Voice Memo Manager for Discord session: ", err)
		return
	}
	session.AddHandler(bot.CommandCenter)

	err = session.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Voice memo bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

type Bot struct {
	GuildSessions    map[string]*GuildSession
	VoiceMemoManager *VoiceMemoManager
}

func NewBot(am *VoiceMemoManager) (*Bot, error) {
	return &Bot{
		GuildSessions:    make(map[string]*GuildSession, 0),
		VoiceMemoManager: am,
	}, nil
}

func (b *Bot) CommandCenter(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself.
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	command := m.Content
	fmt.Println("Message: ", command)

	// Find the channel that the message came from.
	c, err := s.State.Channel(m.ChannelID)
	if err != nil {
		// Could not find channel.
		return
	}

	// Find the guild for that channel.
	g, err := s.State.Guild(c.GuildID)
	if err != nil {
		// Could not find guild.
		return
	}

	if strings.HasPrefix(m.Content, "!") {

		args := strings.Fields(command)
		command = strings.TrimPrefix(args[0], "!")

		switch command {
		case "join":
			b.HandleJoin(s, g, c, m)
		case "leave":
			b.HandleLeave(s, g)
		case "play":
			b.HandlePlay(s, g, c, strings.TrimPrefix(args[1], "-"))
		case "list":
			b.HandleList(s, c)
		case "upload":
		case "record":
		default:
			s.ChannelMessageSend(c.ID, "Unrecognizable command, dummy...")
		}

	}
}

func (b *Bot) HandleJoin(s *discordgo.Session, g *discordgo.Guild, c *discordgo.Channel, m *discordgo.MessageCreate) {
	// Look for Guild Session by id, else create one.
	_, ok := b.GuildSessions[g.ID]
	if ok {
		// Guild session already exists.
		fmt.Println("Already joined a voice channel in ", g.Name)
		s.ChannelMessageSend(c.ID, "I have already joined a voice channel in "+g.Name)
		return
	}

	// Create Guild Session.
	fmt.Println("Creating new Guild session for: ", g.Name)
	b.GuildSessions[g.ID] = &GuildSession{
		ID:        g.ID,
		GuildName: g.Name,
		Session:   s,
	}

	// Look for the message sender in that guild's current voice states.
	fmt.Println("Attempting to join voice channel in ", g.Name)
	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {

			// Then join the channel inside that guild.
			_, err := s.ChannelVoiceJoin(g.ID, vs.ChannelID, false, true)

			if err != nil {
				fmt.Println("Error joining voice channel:", err)
				return
			}
		}
	}
}

func (b *Bot) HandleLeave(s *discordgo.Session, g *discordgo.Guild) {
	gs, ok := b.GuildSessions[g.ID]
	if !ok {
		fmt.Println("Error finding guild session.")
		return
	}

	// Disconnect from channel in guild, then remove guild session.
	gs.Disconnect()
	delete(b.GuildSessions, g.ID)
}

func (b *Bot) HandlePlay(s *discordgo.Session, g *discordgo.Guild, c *discordgo.Channel, fileName string) {
	gs, ok := b.GuildSessions[g.ID]
	if !ok {
		fmt.Println("Error finding guild session.")
		return
	}

	voiceMemo := b.VoiceMemoManager.Get(fileName)
	if voiceMemo == nil {
		fmt.Println("Cannot find ", fileName)
		s.ChannelMessageSend(c.ID, "Cannot find "+fileName)
		return
	}

	gs.Play(voiceMemo)
}

func (b *Bot) HandleList(s *discordgo.Session, c *discordgo.Channel) {
	s.ChannelMessageSend(c.ID, "Here are all voice memos: zollo, drob, mteams, spongebad, djack")
}

type GuildSession struct {
	ID        string
	GuildName string
	Session   *discordgo.Session
	//PlayQueue
}

func (gs *GuildSession) Play(voiceMemo *VoiceMemo) {
	vc := gs.Session.VoiceConnections[gs.ID]

	// Sleep for a specified amount of time before playing the sound.
	time.Sleep(100 * time.Millisecond)

	// Start speaking.
	vc.Speaking(true)

	// Send the buffer data.
	for _, buff := range voiceMemo.buffer {
		vc.OpusSend <- buff
	}

	// Stop speaking.
	vc.Speaking(false)

	// Sleep for a specificed amount of time before ending.
	time.Sleep(100 * time.Millisecond)
}

func (gs *GuildSession) Disconnect() {
	gs.Session.VoiceConnections[gs.ID].Disconnect()
}

type VoiceMemoManager struct {
	Store map[string]*VoiceMemo
	// db instance?
}

func NewVoiceMemoManager() (*VoiceMemoManager, error) {
	voiceMemoMap := make(map[string]*VoiceMemo)

	// Hardcoded for now. Will eventually query from db to get list of voice memos.
	arr := [5]string{"zollo", "drob", "mteams", "spongebad", "djack"}

	// using for loop
	for i := 0; i < len(arr); i++ {
		vm := &VoiceMemo{arr[i], make([][]byte, 0)}
		voiceMemoMap[vm.name] = vm
	}

	m := &VoiceMemoManager{
		Store: voiceMemoMap,
	}
	return m, nil
}

func (m *VoiceMemoManager) LoadAll() (err error) {
	for _, voiceMemo := range m.Store {
		voiceMemo.Load()
	}
	return nil
}

func (m *VoiceMemoManager) Get(fileName string) *VoiceMemo {
	// Try to find voiceMemo file in memory store.
	if file, ok := m.Store[fileName]; ok {
		return file
	}
	return nil
}

type VoiceMemo struct {
	name   string
	buffer [][]byte
}

// Attempts to load an encoded voiceMemo file from disk.
func (vm *VoiceMemo) Load() error {
	extension := ".dca"
	file, err := os.Open("voicememo_files/" + vm.name + extension)
	if err != nil {
		fmt.Println("Error opening dca file :", err)
		return err
	}

	var opuslen int16

	for {
		// Read opus frame length from dca file.
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err := file.Close()
			if err != nil {
				return err
			}
			return nil
		}

		if err != nil {
			fmt.Println("Error reading from dca file1 :", err)
			return err
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors.
		if err != nil {
			fmt.Println("Error reading from dca file2 :", err)
			return err
		}

		// Append encoded pcm data to the buffer.
		vm.buffer = append(vm.buffer, InBuf)
	}
}
