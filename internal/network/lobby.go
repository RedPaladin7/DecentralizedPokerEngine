package network

type LobbyState int 

const (
	LobbyWaiting LobbyState = iota 
	LobbyReady 
	LobbyPlaying
)

type SeatInfo struct {
	PlayerID string 
}