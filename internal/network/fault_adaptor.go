package network

import "github.com/RedPaladin7/DecentralizedPokerEngine/internal/fault"

type GameLogFaultAdaptor struct {
	log *Gamelog
}

func NewGameLogFaultAdaptor(gl *Gamelog) *GameLogFaultAdaptor {
	return &GameLogFaultAdaptor{log: gl}
}

func (a *GameLogFaultAdaptor) DetectEquivocation() (string, *fault.LogEntry, *fault.LogEntry) {
	senderID, envA, envB, _ := a.log.DetectEquivocation()
	if senderID == "" {
		return "", nil, nil
	}
	toEntry := func(e *Envelope) *fault.LogEntry {
		return &fault.LogEntry{
			SenderID: e.SenderId,
			Seq: e.Seq,
			Payload: e.Payload,
			Signature: e.Signature,
		}
	}
	return senderID, toEntry(envA), toEntry(envB)
}

