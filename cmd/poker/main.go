package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RedPaladin7/DecentralizedPokerEngine/config"
	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/fault"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/network"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "keygen":
			runKeygen()
			return
		case "version":
			fmt.Println("p2p-poker v0.7.0 (phase 7 — integration)")
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "host":
			if err := runHost(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "host error: %v\n", err)
				os.Exit(1)
			}
			return
		case "join":
			if err := runJoin(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "join error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run is the entry point for the default local-mode game.
func run() error {
	configPath := ""
	for i, arg := range os.Args[1:] {
		if arg == "--config" || arg == "-c" {
			if i+2 < len(os.Args) {
				configPath = os.Args[i+2]
			}
		}
	}
	cfg, err := config.LoadOrDefault(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return runLocalMode(ctx, cfg)
}

// ─────────────────────────────────────────────────────────────────────────────
// LOCAL MODE (single machine, bot opponents)
// ─────────────────────────────────────────────────────────────────────────────

func runLocalMode(ctx context.Context, cfg *config.Config) error {
	const humanPlayerID = "you"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	players := []*game.Player{
		game.NewPlayer(humanPlayerID, cfg.PlayerName, cfg.Game.BuyIn),
	}
	botNames := []string{"Alice (bot)", "Bob (bot)", "Carol (bot)", "Dave (bot)", "Eve (bot)"}
	for i := 1; i < cfg.Game.MaxSeats; i++ {
		id := fmt.Sprintf("bot-%d", i)
		players = append(players, game.NewPlayer(id, botNames[(i-1)%len(botNames)], cfg.Game.BuyIn))
	}

	dealerIdx := 0
	handNum := 1
	gs := game.NewGameState(cfg.Game.TableID, handNum, players, dealerIdx, cfg.Game.SmallBlind, cfg.Game.BigBlind)
	m := game.NewMachine(gs, rng)

	playerIDs := make([]string, len(players))
	for i, p := range players {
		playerIDs[i] = p.ID
	}
	fm := fault.NewFaultManager(humanPlayerID, int64(handNum), fault.FaultConfig{
		HeartbeatTimeout: cfg.Fault.HeartbeatTimeout,
		VoteExpiry:       cfg.Fault.VoteExpiry,
	})
	fm.RegisterPlayers(playerIDs)
	_ = fm

	var gameModel *localGameModel
	ui := tui.NewModel(humanPlayerID, func(a game.Action) {
		if gameModel != nil {
			gameModel.applyHumanAction(a)
		}
	})
	ui.LobbyStatus = fmt.Sprintf("Local game — %d players — %s/%s blinds",
		cfg.Game.MaxSeats, formatChips(cfg.Game.SmallBlind), formatChips(cfg.Game.BigBlind))

	gameModel = &localGameModel{
		ui: ui, gs: gs, machine: m, players: players,
		dealerIdx: dealerIdx, handNum: handNum, rng: rng, cfg: cfg,
	}

	p := tea.NewProgram(gameModel, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

type localGameModel struct {
	ui        tui.Model
	gs        *game.GameState
	machine   *game.Machine
	players   []*game.Player
	dealerIdx int
	handNum   int
	rng       *rand.Rand
	cfg       *config.Config
}

func (gm *localGameModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		func() tea.Msg {
			if err := gm.machine.StartHand(); err != nil {
				return tui.ErrorMsg{Text: err.Error()}
			}
			return tui.GameStateMsg{State: gm.gs}
		},
	)
}

func (gm *localGameModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.GameStateMsg:
		gm.gs = msg.State
		newUI, _ := gm.ui.Update(msg)
		gm.ui = newUI.(tui.Model)
		if gm.gs.Phase == game.PhaseSettled {
			winnerIDs, handRanks := buildWinnerInfo(gm.gs)
			newUI2, _ := gm.ui.Update(tui.WinnerMsg{WinnerIDs: winnerIDs, HandRanks: handRanks, Payouts: gm.gs.Payouts})
			gm.ui = newUI2.(tui.Model)
			return gm, gm.nextHandCmd()
		}
		if cur := gm.gs.CurrentPlayer(); cur != nil && cur.ID != "you" {
			return gm, gm.botActionCmd()
		}
		return gm, nil
	case tui.ErrorMsg:
		newUI, cmd := gm.ui.Update(msg)
		gm.ui = newUI.(tui.Model)
		return gm, cmd
	case tui.WinnerMsg:
		newUI, cmd := gm.ui.Update(msg)
		gm.ui = newUI.(tui.Model)
		return gm, cmd
	case tea.KeyMsg:
		newUI, cmd := gm.ui.Update(msg)
		gm.ui = newUI.(tui.Model)
		if gm.gs != nil && gm.ui.Mode == tui.ModeSpectate {
			return gm, tea.Batch(cmd, func() tea.Msg { return tui.GameStateMsg{State: gm.gs} })
		}
		return gm, cmd
	default:
		newUI, cmd := gm.ui.Update(msg)
		gm.ui = newUI.(tui.Model)
		return gm, cmd
	}
}

func (gm *localGameModel) View() string { return gm.ui.View() }

func (gm *localGameModel) applyHumanAction(a game.Action) { _ = gm.machine.ApplyAction(a) }

func (gm *localGameModel) botActionCmd() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(_ time.Time) tea.Msg {
		cur := gm.gs.CurrentPlayer()
		if cur == nil || cur.ID == "you" {
			return nil
		}
		toCall := gm.gs.CurrentBet - cur.CurrentBet
		a := game.Action{PlayerID: cur.ID, Type: game.ActionCheck}
		if toCall > 0 {
			a.Type = game.ActionCall
		}
		gm.machine.ApplyAction(a)
		return tui.GameStateMsg{State: gm.gs}
	})
}

func (gm *localGameModel) nextHandCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		gm.handNum++
		gm.dealerIdx = (gm.dealerIdx + 1) % len(gm.players)
		for _, p := range gm.players {
			p.ResetForNewHand()
		}
		gm.gs = game.NewGameState(gm.cfg.Game.TableID, gm.handNum, gm.players,
			gm.dealerIdx, gm.cfg.Game.SmallBlind, gm.cfg.Game.BigBlind)
		gm.machine = game.NewMachine(gm.gs, gm.rng)
		if err := gm.machine.StartHand(); err != nil {
			return tui.ErrorMsg{Text: err.Error()}
		}
		return tui.GameStateMsg{State: gm.gs}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// P2P SUBCOMMAND ENTRY POINTS
// ─────────────────────────────────────────────────────────────────────────────

func applyP2PFlags(cfg *config.Config, args []string) bool {
	noCrypto := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-crypto":
			noCrypto = true
		case "--seats":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &cfg.Game.MaxSeats)
			}
		case "--listen":
			if i+1 < len(args) {
				i++
				cfg.Network.ListenAddr = args[i]
			}
		case "--name":
			if i+1 < len(args) {
				i++
				cfg.PlayerName = args[i]
			}
		case "--table":
			if i+1 < len(args) {
				i++
				cfg.Game.TableID = args[i]
			}
		case "--peer":
			if i+1 < len(args) {
				i++
				cfg.Network.BootstrapPeers = []string{args[i]}
			}
		}
	}
	return noCrypto
}

// runHost: poker host [--seats N] [--name NAME] [--table ID] [--listen ADDR] [--no-crypto]
func runHost(args []string) error {
	cfg, err := config.LoadOrDefault("")
	if err != nil {
		return err
	}
	noCrypto := applyP2PFlags(cfg, args)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return runP2PMode(ctx, cfg, noCrypto)
}

// runJoin: poker join --peer MULTIADDR [--name NAME] [--table ID] [--listen ADDR] [--no-crypto]
func runJoin(args []string) error {
	cfg, err := config.LoadOrDefault("")
	if err != nil {
		return err
	}
	noCrypto := applyP2PFlags(cfg, args)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return runP2PMode(ctx, cfg, noCrypto)
}

// ─────────────────────────────────────────────────────────────────────────────
// runP2PMode — the full multiplayer implementation
// ─────────────────────────────────────────────────────────────────────────────
//
// Design decisions / bug fixes vs the previous broken version:
//
//  1. ALL callbacks are wired BEFORE node.Start() so no messages are silently
//     dropped in the window between Start() launching the receive goroutine
//     and the caller setting the callbacks.
//
//  2. SHARED RNG SEED: every node derives the same int64 seed from
//     Lobby.SessionNonce() — a deterministic concatenation of all peer IDs in
//     join-time order that is identical on every node.  This makes every node
//     shuffle the deck in exactly the same order, so all players see the same
//     hole cards dealt to the correct seats.
//
//  3. ACTION SEQUENCER: GossipSub does not guarantee delivery order.
//     PlayerAction messages carry a Seq field.  The actionSequencer buffers
//     out-of-order messages and releases them in monotonically increasing
//     Seq order so game.Machine.ApplyAction is always called in the right
//     sequence on every node.
//
//  4. MACHINE POINTER INDIRECTION: when a new hand starts, a new
//     game.Machine is created.  The network callback goroutine must see the
//     updated pointer.  We use a *(*game.Machine) with a mutex so the callback
//     always operates on the current hand's machine.
//
//  5. LOBBY MAX SEATS: node.go was hardcoding NewLobby(tableID, 9) regardless
//     of the configured MaxSeats — fixed in node.go.

func runP2PMode(ctx context.Context, cfg *config.Config, noCrypto bool) error {

	// ── Identity + crypto keys ────────────────────────────────────────────────
	seed, err := cfg.LoadIdentityKey()
	if err != nil {
		return fmt.Errorf("identity key: %w", err)
	}
	prime := pokercrypto.SharedPrime()
	var sraKey *pokercrypto.SRAKey
	if !noCrypto {
		sraKey, err = pokercrypto.GenerateSRAKey(prime)
		if err != nil {
			return fmt.Errorf("SRA key: %w", err)
		}
	}

	// ── Action sequencer (shared between callback goroutine and TUI) ──────────
	seq := &actionSequencer{nextSeq: 1, pending: make(map[int64]*network.PlayerAction)}

	// ── Mutable machine pointers, guarded by machineMu ───────────────────────
	// The network callback goroutine and the TUI goroutine both access these.
	var machineMu sync.Mutex
	var liveMachine *game.Machine
	var liveGS *game.GameState

	// prog is set after tea.NewProgram; used to push state updates into the TUI.
	var prog *tea.Program

	// ── Build node — callbacks set BEFORE Start() ─────────────────────────────
	node, err := network.NewNode(
		ctx,
		cfg.Game.TableID,
		cfg.PlayerName,
		cfg.Game.BuyIn,
		cfg.Game.MaxSeats,
		sraKey,
		cfg.Network.ListenAddr,
		seed,
		cfg.Network.BootstrapPeers...,
	)
	if err != nil {
		return fmt.Errorf("new node: %w", err)
	}

	// Lobby notifications (printed to stdout before TUI starts).
	node.OnJoinTable = func(msg *network.JoinTable, from string) {
		fmt.Printf("[lobby] %-20s joined   (%d / %d seats)\n",
			msg.PlayerName, node.Lobby.Count(), cfg.Game.MaxSeats)
	}
	node.OnPlayerReady = func(msg *network.PlayerReady, from string) {
		fmt.Printf("[lobby] %s ready\n", from)
	}

	// Remote action handler — runs in the network receive goroutine.
	// FIX 3: use sequencer to enforce ordering.
	// FIX 4: read liveMachine under machineMu so new-hand swaps are visible.
	node.OnPlayerAction = func(msg *network.PlayerAction) {
		machineMu.Lock()
		m := liveMachine
		gs := liveGS
		if m == nil {
			machineMu.Unlock()
			return
		}
		ready := seq.push(msg)
		for _, rm := range ready {
			a := game.Action{
				PlayerID: rm.PlayerId,
				Type:     game.ActionType(rm.Action),
				Amount:   rm.Amount,
			}
			_ = m.ApplyAction(a)
		}
		machineMu.Unlock()
		if prog != nil && len(ready) > 0 {
			prog.Send(tui.GameStateMsg{State: gs})
		}
	}

	// Heartbeat handler — placeholder, updated later
	node.OnHeartbeat = func(msg *network.Heartbeat, sender string) { /* updated later */ }

	// Hand result handler — for verification (basic: just log)
	node.OnHandResult = func(msg *network.HandResult) {
		fmt.Printf("[hand result] hand %d settled\n", msg.HandNum)
	}

	// Crypto callbacks MUST be set before Start so early SHUFFLE_STEP /
	// PARTIAL_DECRYPT are not dropped. The session exists only after lobby fill.
	var cryptoMu sync.Mutex
	var liveHand *network.CryptoHand
	var earlyShuffle []*network.ShuffleStep
	var earlyPeels []*network.PartialDecrypt

	node.OnShuffleStep = func(pb *network.ShuffleStep) {
		cryptoMu.Lock()
		h := liveHand
		if h == nil {
			if len(earlyShuffle) < 16 {
				earlyShuffle = append(earlyShuffle, pb)
			}
			cryptoMu.Unlock()
			return
		}
		cryptoMu.Unlock()
		out, err := h.HandleShuffle(network.ShuffleMessageFromWire(pb))
		if err != nil {
			fmt.Printf("[crypto] HandleShuffle: %v\n", err)
			return
		}
		for _, msg := range out {
			if err := node.BroadcastShuffleMessage(ctx, msg); err != nil {
				fmt.Printf("[error] broadcast shuffle: %v\n", err)
			}
		}
	}
	node.OnPartialDecrypt = func(pb *network.PartialDecrypt) {
		cryptoMu.Lock()
		h := liveHand
		if h == nil {
			if len(earlyPeels) < 32 {
				earlyPeels = append(earlyPeels, pb)
			}
			cryptoMu.Unlock()
			return
		}
		cryptoMu.Unlock()
		out, err := h.HandlePeel(network.PeelMessageFromWire(pb))
		if err != nil {
			fmt.Printf("[crypto] HandlePeel: %v\n", err)
			return
		}
		for _, msg := range out {
			if err := node.SendPeel(ctx, msg); err != nil {
				fmt.Printf("[error] send peel: %v\n", err)
			}
		}
	}

	installHand := func(h *network.CryptoHand) {
		cryptoMu.Lock()
		liveHand = h
		shuffles := earlyShuffle
		peels := earlyPeels
		earlyShuffle = nil
		earlyPeels = nil
		cryptoMu.Unlock()
		for _, pb := range shuffles {
			out, err := h.HandleShuffle(network.ShuffleMessageFromWire(pb))
			if err != nil {
				fmt.Printf("[crypto] drain shuffle: %v\n", err)
				continue
			}
			for _, msg := range out {
				_ = node.BroadcastShuffleMessage(ctx, msg)
			}
		}
		for _, pb := range peels {
			out, err := h.HandlePeel(network.PeelMessageFromWire(pb))
			if err != nil {
				fmt.Printf("[crypto] drain peel: %v\n", err)
				continue
			}
			for _, msg := range out {
				_ = node.SendPeel(ctx, msg)
			}
		}
	}

	// ── Start networking ──────────────────────────────────────────────────────
	if err := node.Start(ctx); err != nil {
		return fmt.Errorf("node start: %w", err)
	}
	defer node.Close()

	localPeerID := node.Host.PeerID

	fmt.Printf("\n=== P2P Poker  ·  %s ===\n", cfg.PlayerName)
	fmt.Printf("Peer ID  : %s\n\n", localPeerID)
	fmt.Println("Share one of these addresses with the other player:")
	for _, a := range node.Host.Addrs() {
		fmt.Printf("  %s\n", a)
	}
	fmt.Printf("\nTable : %s   Seats : %d   Buy-in : %d chips\n",
		cfg.Game.TableID, cfg.Game.MaxSeats, cfg.Game.BuyIn)
	fmt.Println()
	fmt.Println("Waiting for players… (Ctrl-C to quit)")
	fmt.Println()

	// ── Broadcast join (retry briefly while mesh forms) ───────────────────────
	for attempt := 0; attempt < 6; attempt++ {
		if err := node.BroadcastJoin(ctx, 1); err != nil {
			fmt.Printf("[error] broadcast join attempt %d: %v\n", attempt+1, err)
		} else {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}

	// ── Wait for lobby to fill ────────────────────────────────────────────────
	pollTick := time.NewTicker(250 * time.Millisecond)
	defer pollTick.Stop()
waitLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pollTick.C:
			if node.Lobby.Count() >= cfg.Game.MaxSeats {
				break waitLoop
			}
		}
	}

	fmt.Printf("\nAll %d players present. Broadcasting ready signal…\n", cfg.Game.MaxSeats)
	if err := node.BroadcastReady(ctx, 1); err != nil {
		fmt.Printf("[error] broadcast ready: %v\n", err)
	}
	// Small pause so every node receives each other's ready broadcast.
	time.Sleep(2 * time.Second)

	nonce := node.Lobby.SessionNonce()
	sharedSeed := int64(0)
	for i, b := range nonce {
		sharedSeed ^= int64(b) << (uint(i%8) * 8)
	}
	sharedSeed = sharedSeed*6364136223846793005 + 1442695040888963407

	seats := node.Lobby.Seats()
	players := make([]*game.Player, len(seats))
	for i, s := range seats {
		players[i] = game.NewPlayer(s.PlayerID, s.PlayerName, s.BuyIn)
	}

	handNum := 1
	dealerIdx := 0

	var kr *pokercrypto.Keyring
	var gs *game.GameState
	var machine *game.Machine

	if noCrypto {
		fmt.Println("DEBUG  ·  --no-crypto  ·  shared-seed plaintext  ·  all cards visible")
		gs = game.NewGameState(cfg.Game.TableID, handNum, players, dealerIdx,
			cfg.Game.SmallBlind, cfg.Game.BigBlind)
		machine = game.NewMachine(gs, rand.New(rand.NewSource(sharedSeed)))
	} else {
		if !node.Lobby.AllSeatsHavePublicE() {
			return fmt.Errorf("runP2PMode: crypto dealing requires every seat to publish e; a peer joined with --no-crypto")
		}
		var err error
		kr, err = network.KeyringFromLobby(localPeerID, sraKey, node.Lobby)
		if err != nil {
			return fmt.Errorf("runP2PMode: %w", err)
		}
		fmt.Println("Cryptographic dealing  ·  SRA 2048-bit  ·  opponent holes stay hidden")
		fmt.Println("Shuffling…")
		machine, gs, liveHand, err = dealCryptoHand(ctx, node, kr, nonce, handNum, dealerIdx, players, localPeerID, cfg, installHand)
		if err != nil {
			return fmt.Errorf("runP2PMode: %w", err)
		}
		fmt.Println("Hole cards dealt. Starting table…")
	}

	machineMu.Lock()
	liveMachine = machine
	liveGS = gs
	machineMu.Unlock()

	// ── Fault manager ─────────────────────────────────────────────────────────
	fm := fault.NewFaultManager(localPeerID, int64(handNum), fault.FaultConfig{
		HeartbeatTimeout: cfg.Fault.HeartbeatTimeout,
		VoteExpiry:       cfg.Fault.VoteExpiry,
		Prime:            prime,
	})
	fm.RegisterPlayers(node.Lobby.CanonicalPlayerOrder())

	// Update heartbeat handler to use fm
	node.OnHeartbeat = func(msg *network.Heartbeat, sender string) {
		fm.RecordHeartbeat(sender)
	}

	// ── Build and run TUI ─────────────────────────────────────────────────────
	model := &p2pGameModel{
		localPeerID: localPeerID,
		players:     players,
		dealerIdx:   dealerIdx,
		handNum:     handNum,
		sharedSeed:  sharedSeed,
		noCrypto:    noCrypto,
		keyring:     kr,
		lobbyNonce:  nonce,
		node:        node,
		ctx:         ctx,
		cfg:         cfg,
		fm:          fm,
		machineMu:   &machineMu,
		machinePtr:  &liveMachine,
		gsPtr:       &liveGS,
		cryptoMu:    &cryptoMu,
		liveHand:    &liveHand,
		installHand: installHand,
		seq:         seq,
		notifyCh:    make(chan struct{}, 16),
	}
	model.fm.OnPlayerFolded = model.forceFold

	// Augment the action callback to also poke the TUI notify channel.
	prevOnAction := node.OnPlayerAction
	node.OnPlayerAction = func(msg *network.PlayerAction) {
		prevOnAction(msg)
		model.kickCryptoAdvance()
		select {
		case model.notifyCh <- struct{}{}:
		default:
		}
	}

	ui := tui.NewModel(localPeerID, func(a game.Action) {
		model.applyAndBroadcast(a)
	})
	ui.LobbyStatus = fmt.Sprintf("P2P · %d players · %s/%s blinds",
		len(players), formatChips(cfg.Game.SmallBlind), formatChips(cfg.Game.BigBlind))
	model.ui = ui

	prog = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	model.prog = prog

	// Heartbeat sender goroutine.
	go func() {
		hs := fault.NewHeartbeatSender(localPeerID, cfg.Fault.HeartbeatInterval,
			func(hbSeq int64) error {
				return node.BroadcastHeartbeat(ctx, int64(handNum), hbSeq)
			})
		_ = hs.Run(ctx)
	}()

	_, runErr := prog.Run()
	return runErr
}

// ─────────────────────────────────────────────────────────────────────────────
// actionSequencer
// ─────────────────────────────────────────────────────────────────────────────
// Buffers PlayerAction messages delivered out of order by GossipSub and
// releases them in ascending Seq order so game.Machine.ApplyAction is always
// called in the correct sequence on every node.

type actionSequencer struct {
	mu      sync.Mutex
	nextSeq int64
	pending map[int64]*network.PlayerAction
}

// push adds msg and returns all messages that are now in-order.
func (s *actionSequencer) push(msg *network.PlayerAction) []*network.PlayerAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[msg.Seq] = msg
	var out []*network.PlayerAction
	for {
		if m, ok := s.pending[s.nextSeq]; ok {
			out = append(out, m)
			delete(s.pending, s.nextSeq)
			s.nextSeq++
		} else {
			break
		}
	}
	return out
}

// reset clears state between hands.
func (s *actionSequencer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSeq = 1
	s.pending = make(map[int64]*network.PlayerAction)
}

// ─────────────────────────────────────────────────────────────────────────────
// p2pGameModel  (implements tea.Model)
// ─────────────────────────────────────────────────────────────────────────────

type p2pGameModel struct {
	ui          tui.Model
	localPeerID string
	players     []*game.Player
	dealerIdx   int
	handNum     int
	sharedSeed  int64
	noCrypto    bool
	keyring     *pokercrypto.Keyring
	lobbyNonce  []byte

	node *network.Node
	ctx  context.Context
	cfg  *config.Config
	fm   *fault.FaultManager
	prog *tea.Program

	// machineMu guards machinePtr and gsPtr.
	// The network receive goroutine reads these; startNextHand writes them.
	machineMu  *sync.Mutex
	machinePtr **game.Machine
	gsPtr      **game.GameState

	cryptoMu    *sync.Mutex
	liveHand    **network.CryptoHand
	installHand func(*network.CryptoHand)

	seq      *actionSequencer
	notifyCh chan struct{}
}

// applyAndBroadcast is the OnAction callback wired into the TUI.
// Called on the TUI goroutine when the local player confirms an action.
func (m *p2pGameModel) applyAndBroadcast(a game.Action) {
	m.machineMu.Lock()
	machine := *m.machinePtr
	gs := *m.gsPtr
	if machine == nil {
		m.machineMu.Unlock()
		return
	}

	m.seq.mu.Lock()
	outSeq := m.seq.nextSeq
	m.seq.nextSeq++
	m.seq.mu.Unlock()

	_ = machine.ApplyAction(a)
	m.machineMu.Unlock()

	if err := m.node.BroadcastAction(m.ctx, int64(m.handNum), a, outSeq); err != nil {
		fmt.Printf("[error] broadcast action: %v\n", err)
	}
	m.kickCryptoAdvance()

	if m.prog != nil {
		m.prog.Send(tui.GameStateMsg{State: gs})
	}
}

func (m *p2pGameModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		func() tea.Msg {
			m.machineMu.Lock()
			machine := *m.machinePtr
			gs := *m.gsPtr
			m.machineMu.Unlock()
			if m.noCrypto {
				if err := machine.StartHand(); err != nil {
					return tui.ErrorMsg{Text: err.Error()}
				}
			}
			return tui.GameStateMsg{State: gs}
		},
		m.waitForUpdate(),
	)
}

// waitForUpdate blocks briefly waiting for a network-driven state change,
// then returns the current game state so the TUI refreshes.
func (m *p2pGameModel) waitForUpdate() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.notifyCh:
		case <-time.After(250 * time.Millisecond):
		case <-m.ctx.Done():
			return nil
		}
		m.machineMu.Lock()
		gs := *m.gsPtr
		m.machineMu.Unlock()
		return tui.GameStateMsg{State: gs}
	}
}

func (m *p2pGameModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tui.GameStateMsg:
		newUI, _ := m.ui.Update(msg)
		m.ui = newUI.(tui.Model)
		if msg.State != nil && msg.State.Phase == game.PhaseSettled {
			winnerIDs, handRanks := buildWinnerInfo(msg.State)
			newUI2, _ := m.ui.Update(tui.WinnerMsg{
				WinnerIDs: winnerIDs,
				HandRanks: handRanks,
				Payouts:   msg.State.Payouts,
			})
			m.ui = newUI2.(tui.Model)
			// Broadcast hand result
			pots := []*network.PotResult{}
			for _, pot := range msg.State.Pots {
				winners := []string{}
				for _, id := range pot.EligibleIDs {
					if msg.State.Payouts[id] > 0 {
						winners = append(winners, id)
					}
				}
				pots = append(pots, &network.PotResult{
					Amount:    pot.Amount,
					WinnerIds: winners,
				})
			}
			if err := m.node.BroadcastHandResult(m.ctx, int64(m.handNum), pots, []byte{}); err != nil {
				fmt.Printf("[error] broadcasting hand result: %v\n", err)
			}
			return m, tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
				return m.startNextHand()
			})
		}
		return m, m.waitForUpdate()

	case tui.ErrorMsg:
		newUI, cmd := m.ui.Update(msg)
		m.ui = newUI.(tui.Model)
		return m, cmd

	case tui.WinnerMsg:
		newUI, cmd := m.ui.Update(msg)
		m.ui = newUI.(tui.Model)
		return m, cmd

	case tea.KeyMsg:
		newUI, cmd := m.ui.Update(msg)
		m.ui = newUI.(tui.Model)
		return m, tea.Batch(cmd, m.waitForUpdate())

	default:
		newUI, cmd := m.ui.Update(msg)
		m.ui = newUI.(tui.Model)
		return m, cmd
	}
}

// startNextHand creates a new game.Machine for the next hand, atomically
// swaps the live pointers so the network callback goroutine sees the new
// machine immediately, and starts the hand.
func (m *p2pGameModel) startNextHand() tea.Msg {
	m.handNum++
	m.dealerIdx = (m.dealerIdx + 1) % len(m.players)
	for _, p := range m.players {
		p.ResetForNewHand()
	}

	m.seq.reset()
	m.node.Lobby.Reset()

	var gs *game.GameState
	var machine *game.Machine

	if m.noCrypto {
		nextSeed := m.sharedSeed ^ int64(m.handNum)*2654435761
		rng := rand.New(rand.NewSource(nextSeed))
		gs = game.NewGameState(m.cfg.Game.TableID, m.handNum, m.players,
			m.dealerIdx, m.cfg.Game.SmallBlind, m.cfg.Game.BigBlind)
		machine = game.NewMachine(gs, rng)
		m.machineMu.Lock()
		*m.machinePtr = machine
		*m.gsPtr = gs
		m.machineMu.Unlock()
		if err := machine.StartHand(); err != nil {
			return tui.ErrorMsg{Text: err.Error()}
		}
		return tui.GameStateMsg{State: gs}
	}

	var err error
	machine, gs, _, err = dealCryptoHand(m.ctx, m.node, m.keyring, m.lobbyNonce, m.handNum, m.dealerIdx, m.players, m.localPeerID, m.cfg, m.installHand)
	if err != nil {
		return tui.ErrorMsg{Text: err.Error()}
	}
	m.machineMu.Lock()
	*m.machinePtr = machine
	*m.gsPtr = gs
	m.machineMu.Unlock()
	return tui.GameStateMsg{State: gs}
}

func (m *p2pGameModel) View() string { return m.ui.View() }

func (m *p2pGameModel) forceFold(peerID string) {
	m.machineMu.Lock()
	machine := *m.machinePtr
	if machine == nil {
		m.machineMu.Unlock()
		return
	}
	for _, p := range machine.State.Players {
		if p.ID == peerID {
			a := game.Action{PlayerID: peerID, Type: game.ActionFold}
			_ = machine.ApplyAction(a)
			if m.prog != nil {
				m.prog.Send(tui.GameStateMsg{State: machine.State})
			}
			break
		}
	}
	m.machineMu.Unlock()
	m.kickCryptoAdvance()
}

func (m *p2pGameModel) kickCryptoAdvance() {
	if m.noCrypto || m.liveHand == nil {
		return
	}
	go func() {
		m.cryptoMu.Lock()
		h := *m.liveHand
		m.cryptoMu.Unlock()
		if h == nil {
			return
		}
		m.machineMu.Lock()
		defer m.machineMu.Unlock()
		machine := *m.machinePtr
		gs := *m.gsPtr
		if machine == nil {
			return
		}
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		err := network.AdvanceCrypto(ctx, h, machine, func(msgs []*pokercrypto.PeelMessage) error {
			for _, msg := range msgs {
				if err := m.node.SendPeel(m.ctx, msg); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil && m.prog != nil {
			m.prog.Send(tui.ErrorMsg{Text: err.Error()})
			return
		}
		if m.prog != nil {
			m.prog.Send(tui.GameStateMsg{State: gs})
		}
	}()
}

const cryptoDealTimeout = 2 * time.Minute

func dealCryptoHand(
	ctx context.Context,
	node *network.Node,
	kr *pokercrypto.Keyring,
	nonce []byte,
	handNum int,
	dealerIdx int,
	players []*game.Player,
	localID string,
	cfg *config.Config,
	install func(*network.CryptoHand),
) (*game.Machine, *game.GameState, *network.CryptoHand, error) {
	h, err := network.NewCryptoHand(kr, nonce, int64(handNum), dealerIdx)
	if err != nil {
		return nil, nil, nil, err
	}
	if install != nil {
		install(h)
	}
	waitCtx, cancel := context.WithTimeout(ctx, cryptoDealTimeout)
	defer cancel()

	outs, err := h.StartShuffle()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, msg := range outs {
		if err := node.BroadcastShuffleMessage(ctx, msg); err != nil {
			fmt.Printf("[error] broadcast shuffle: %v\n", err)
		}
	}
	if err := h.WaitShuffle(waitCtx); err != nil {
		return nil, nil, nil, err
	}

	peels, err := h.StartHoles()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, msg := range peels {
		if err := node.SendPeel(ctx, msg); err != nil {
			fmt.Printf("[error] send peel: %v\n", err)
		}
	}
	if err := h.WaitHoles(waitCtx); err != nil {
		return nil, nil, nil, err
	}

	holes, err := h.LocalHoles()
	if err != nil {
		return nil, nil, nil, err
	}

	gs := game.NewGameState(cfg.Game.TableID, handNum, players, dealerIdx,
		cfg.Game.SmallBlind, cfg.Game.BigBlind)
	for _, p := range gs.Players {
		if p.ID == localID {
			p.HoleCards = holes
		} else {
			p.HoleCards = [2]game.Card{}
		}
	}
	machine := game.NewMachine(gs, nil)
	if err := machine.StartHandCrypto(); err != nil {
		return nil, nil, nil, err
	}
	return machine, gs, h, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared helpers
// ─────────────────────────────────────────────────────────────────────────────

func buildWinnerInfo(gs *game.GameState) (map[string]bool, map[string]string) {
	winnerIDs := make(map[string]bool)
	handRanks := make(map[string]string)
	for id, payout := range gs.Payouts {
		if payout > 0 {
			winnerIDs[id] = true
		}
	}
	if len(gs.CommunityCards) == 5 {
		for _, p := range gs.Players {
			if p.Status != game.StatusFolded && winnerIDs[p.ID] {
				var seven [7]game.Card
				seven[0] = p.HoleCards[0]
				seven[1] = p.HoleCards[1]
				for i, c := range gs.CommunityCards {
					seven[i+2] = c
				}
				h := game.EvaluateBest7(seven)
				handRanks[p.ID] = h.Rank.String()
			}
		}
	}
	return winnerIDs, handRanks
}

// ─────────────────────────────────────────────────────────────────────────────
// CLI helpers
// ─────────────────────────────────────────────────────────────────────────────

func runInit() {
	path := "config.yaml"
	if _, err := os.Stat(path); err == nil {
		fmt.Println("config.yaml already exists. Remove it first to reinitialise.")
		return
	}
	if err := os.WriteFile(path, []byte(config.DefaultYAML()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ config.yaml created. Edit it then run: poker")
}

func runKeygen() {
	_, hexKey, err := config.GenerateECDSAKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "keygen error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("New Ethereum private key:\n  private_key_hex: \"%s\"\n\n", hexKey)
	fmt.Println("Add this to config.yaml under 'chain:'. KEEP IT SECRET.")
}

func printHelp() {
	fmt.Print(`P2P Texas Hold'em Poker Engine

USAGE:
  poker                    Local game vs bots (reads config.yaml)
  poker host [flags]       Host a multiplayer table
  poker join [flags]       Join an existing table
  poker init               Generate a default config.yaml
  poker keygen             Generate a new Ethereum private key
  poker version            Print version

FLAGS:
  --seats N                Max seats (host only)
  --name NAME              Player name
  --table ID               Table ID
  --listen ADDR            Listen address
  --no-crypto              Debug: shared-seed plaintext dealing (all cards visible; sync testing only)

HOST FLAGS:
  --seats N                Number of seats (2-9, default: from config)
  --name  NAME             Your display name
  --table ID               Table identifier string
  --listen ADDR            libp2p listen address (default /ip4/0.0.0.0/tcp/9000)

JOIN FLAGS:
  --peer  MULTIADDR        Host multiaddr printed by 'poker host'
  --name  NAME             Your display name
  --table ID               Must match the host's table ID
  --listen ADDR            Your libp2p listen address

EXAMPLES:

  # Two players on the same LAN (MDNS finds each other automatically):
  #   Machine A:
  poker host --seats 2 --name Alice --table friday

  #   Machine B:
  poker join --name Bob --table friday

  # Two players on different networks (copy the multiaddr from host output):
  #   Machine A:
  poker host --seats 2 --name Alice --table friday

  #   Machine B:
  poker join --name Bob --table friday \
    --peer /ip4/1.2.3.4/tcp/9000/p2p/12D3KooW...

KEYBOARD CONTROLS:
  f          Fold
  c          Check / Call
  r          Raise (opens amount input)
  a          All-in
  ←/→ h/l   Navigate action buttons
  Enter      Confirm action
  ↑/↓ k/j   Scroll log
  q          Quit
`)
}

func formatChips(chips int64) string {
	if chips >= 1000 {
		return fmt.Sprintf("%dk", chips/1000)
	}
	return fmt.Sprintf("%d", chips)
}
